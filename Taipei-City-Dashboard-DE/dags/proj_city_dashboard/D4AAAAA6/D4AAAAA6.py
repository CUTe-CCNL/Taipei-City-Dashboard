from airflow import DAG
from operators.common_pipeline import CommonDag


def etl_function(**kwargs):
    import json
    import math
    import os
    from pathlib import Path
    import pandas as pd
    import requests
    from sqlalchemy import create_engine
    from utils.load_stage import save_dataframe_to_postgresql

    def _serialize_property_value(value):
        if pd.isna(value):
            return None
        if isinstance(value, pd.Timestamp):
            return value.isoformat()
        return value

    def _validate_coordinates(lng, lat):
        if not (isinstance(lng, (int, float)) and isinstance(lat, (int, float))):
            return False
        if not (math.isfinite(float(lng)) and math.isfinite(float(lat))):
            return False
        return -180 <= float(lng) <= 180 and -90 <= float(lat) <= 90

    def _build_geojson_feature_collection(df):
        feature_rows = df.dropna(subset=["lng", "lat"]).copy()
        feature_rows = feature_rows[
            feature_rows.apply(
                lambda row: _validate_coordinates(row["lng"], row["lat"]), axis=1
            )
        ]
        features = []
        for _, row in feature_rows.iterrows():
            properties = {}
            for key in ["_id", "event_time", "case_type", "location", "data_time"]:
                properties[key] = _serialize_property_value(row.get(key))

            features.append(
                {
                    "type": "Feature",
                    "properties": properties,
                    "geometry": {
                        "type": "Point",
                        "coordinates": [float(row["lng"]), float(row["lat"])],
                    },
                }
            )

        return {
            "type": "FeatureCollection",
            "crs": {
                "type": "name",
                "properties": {"name": "urn:ogc:def:crs:OGC:1.3:CRS84"},
            },
            "features": features,
        }

    def _validate_geojson_feature_collection(geojson_obj):
        if not isinstance(geojson_obj, dict):
            raise ValueError("GeoJSON should be a dictionary object.")
        if geojson_obj.get("type") != "FeatureCollection":
            raise ValueError("GeoJSON type should be FeatureCollection.")
        if not isinstance(geojson_obj.get("features"), list):
            raise ValueError("GeoJSON features should be a list.")

        for feature in geojson_obj["features"]:
            if feature.get("type") != "Feature":
                raise ValueError("GeoJSON feature type should be Feature.")
            geometry = feature.get("geometry", {})
            if geometry.get("type") != "Point":
                raise ValueError("GeoJSON geometry type should be Point.")
            coordinates = geometry.get("coordinates")
            if not (
                isinstance(coordinates, list)
                and len(coordinates) == 2
                and _validate_coordinates(coordinates[0], coordinates[1])
            ):
                raise ValueError("GeoJSON point coordinates are invalid.")
            if not isinstance(feature.get("properties"), dict):
                raise ValueError("GeoJSON properties should be a dictionary.")

    ready_data_db_uri = kwargs.get("ready_data_db_uri")
    proxies = kwargs.get("proxies")
    dag_infos = kwargs.get("dag_infos")
    load_behavior = dag_infos.get("load_behavior")
    default_table = dag_infos.get("ready_data_default_table")
    history_table = dag_infos.get("ready_data_history_table")

    resource_id = "d4aaaaa6-d03e-4539-945b-cdbd9387007d"
    q = ""
    limit = 1000
    offset = 0
    timeout = 60

    if limit > 1000:
        raise ValueError("limit should be <= 1000 for data.taipei API.")
    if offset < 0:
        raise ValueError("offset should be >= 0.")

    base_url = f"https://data.taipei/api/v1/dataset/{resource_id}"
    request_params = {"scope": "resourceAquire", "resource_id": resource_id}
    if q:
        request_params["q"] = q

    first_page_params = {**request_params, "limit": limit, "offset": offset}
    first_page_resp = requests.get(
        base_url,
        params=first_page_params,
        proxies=proxies,
        timeout=timeout,
    )
    first_page_resp.raise_for_status()
    first_page_json = first_page_resp.json()

    total_count = int(first_page_json.get("result", {}).get("count", 0))
    all_rows = first_page_json.get("result", {}).get("results", [])

    next_offset = offset + limit
    while next_offset < total_count:
        page_params = {**request_params, "limit": limit, "offset": next_offset}
        page_resp = requests.get(
            base_url,
            params=page_params,
            proxies=proxies,
            timeout=timeout,
        )
        page_resp.raise_for_status()
        page_rows = page_resp.json().get("result", {}).get("results", [])
        all_rows.extend(page_rows)
        if not page_rows:
            break
        next_offset += limit

    raw_data = pd.DataFrame(all_rows)
    if raw_data.empty:
        raise ValueError("No data fetched from data.taipei API.")

    data = raw_data.copy()
    data["data_time"] = pd.to_datetime(
        data["_importdate"].apply(lambda x: x.get("date") if isinstance(x, dict) else None),
        errors="coerce",
    )
    data = data.rename(
        columns={
            "發生時間": "event_time",
            "處理別": "case_type",
            "肇事地點": "location",
            "座標－x": "lng",
            "座標－y": "lat",
        }
    )
    data["event_time"] = pd.to_datetime(data["event_time"], errors="coerce")
    data["lng"] = pd.to_numeric(data["lng"], errors="coerce")
    data["lat"] = pd.to_numeric(data["lat"], errors="coerce")
    ready_data = data[
        [
            "_id",
            "event_time",
            "case_type",
            "location",
            "lng",
            "lat",
            "data_time",
        ]
    ]

    engine = create_engine(ready_data_db_uri)
    save_dataframe_to_postgresql(
        engine=engine,
        data=ready_data,
        load_behavior=load_behavior,
        default_table=default_table,
        history_table=history_table,
    )

    geojson_obj = _build_geojson_feature_collection(ready_data)
    _validate_geojson_feature_collection(geojson_obj)

    output_dir = (
        dag_infos.get("geojson_output_dir")
        or os.getenv("MAPBOX_GEOJSON_OUTPUT_DIR")
        or "/opt/airflow/data/mapData"
    )
    fe_geojson_path = Path(output_dir)
    output_filename = dag_infos.get("geojson_filename", "traffic_accident_events_tpe.geojson")
    output_path = fe_geojson_path / output_filename
    output_path.parent.mkdir(parents=True, exist_ok=True)
    with output_path.open("w", encoding="utf-8") as fp:
        json.dump(geojson_obj, fp, ensure_ascii=False, indent=2)

dag = CommonDag(proj_folder="proj_city_dashboard", dag_folder="D4AAAAA6")
dag.create_dag(etl_func=etl_function)

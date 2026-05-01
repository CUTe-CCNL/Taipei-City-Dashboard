from airflow import DAG
from operators.common_pipeline import CommonDag

def etl_function(**kwargs):
    """
    針對「臺北市即時交通訊息」(128476) 設計的 ETL。
    此資料集為純文本訊息，無地理空間資訊(Geometry)。
    """
    import requests
    import pandas as pd
    from utils.transform_time import convert_str_to_time_format
    from utils.load_stage import update_lasttime_in_data_to_dataset_info
    from sqlalchemy import create_engine

    # 1. Config 獲取
    ready_data_db_uri = kwargs.get("ready_data_db_uri")
    dag_infos = kwargs.get("dag_infos")
    dag_id = dag_infos.get("dag_id")
    load_behavior = dag_infos.get("load_behavior")
    default_table = dag_infos.get("ready_data_default_table")
    history_table = dag_infos.get("ready_data_history_table")
    
    # 臺北市即時交通訊息的實際 JSON 實體檔案網址
    DATA_URL = "https://tcgbusfs.blob.core.windows.net/dotapp/news.json"

    # 2. Extract
    try:
        res = requests.get(DATA_URL).json()
        # 根據我們看到的結構，資料包在 "News" 陣列中
        raw_data = pd.DataFrame(res.get("News", []))
    except Exception as e:
        print(f"獲取或解析資料失敗: {e}")
        raw_data = pd.DataFrame()

    # 防呆：如果沒抓到資料就提早結束
    if raw_data.empty:
        print("未獲取到任何資料，結束 ETL 流程。")
        return

    # 3. Transform
    data = raw_data.copy()
    data.columns = data.columns.str.lower()

    # 確保所有預期欄位存在，避免資料庫 Schema 對不起來
    expected_columns = ["chtmessage", "engmessage", "starttime", "endtime", "updatetime", "content", "url", "areaname"]
    for col in expected_columns:
        if col not in data.columns:
            data[col] = None

    # 時間格式標準化 (將字串轉為帶有時區的 datetime)
    data["starttime"] = convert_str_to_time_format(data["starttime"])
    data["endtime"] = convert_str_to_time_format(data["endtime"])
    data["updatetime"] = convert_str_to_time_format(data["updatetime"])
    
    # 新增系統資料時間，做為後續追蹤基準
    data["data_time"] = data["updatetime"]

    # 挑選最終需要的欄位 (去除了 wkb_geometry 等空間欄位)
    ready_data = data[["data_time", "chtmessage", "engmessage", "starttime", "endtime", "content", "url", "areaname"]]

    # 4. Load
    engine = create_engine(ready_data_db_uri)
    
    # 根據 load_behavior 決定寫入策略 (使用 Pandas 原生的 to_sql)
    if load_behavior in ["current", "current+history", "replace"]:
        ready_data.to_sql(
            default_table, 
            engine, 
            if_exists='replace', 
            index=False
        )
    
    if load_behavior in ["history", "current+history"] and history_table:
        ready_data.to_sql(
            history_table, 
            engine, 
            if_exists='append', 
            index=False
        )

    # 5. Update Metadata
    lasttime_in_data = ready_data["data_time"].max()
    update_lasttime_in_data_to_dataset_info(
        engine, airflow_dag_id=dag_id, lasttime_in_data=lasttime_in_data
    )


# 6. 初始化 DAG (這裡已填入你正確的資料夾路徑)
dag = CommonDag(proj_folder="proj_city_dashboard", dag_folder="a_taipei_traffic_news")
dag.create_dag(etl_func=etl_function)
import os
import time

import pandas as pd
import requests
from airflow import DAG
from airflow.models import Variable
from sqlalchemy import create_engine

from operators.common_pipeline import CommonDag
from utils.load_stage import save_dataframe_to_postgresql

def _transfer(**kwargs):
    """
    MOENV stat_p_45 (一般廢棄物產生量及處理狀況) -> PostgreSQL
    過濾：臺北市、新北市
    """
    # --- 執行環境與 DAG 目標表（由 job_config 注入）---
    ready_data_db_uri = kwargs.get("ready_data_db_uri")
    proxies = kwargs.get("proxies")
    dag_infos = kwargs.get("dag_infos")
    load_behavior = dag_infos.get("load_behavior")
    default_table = dag_infos.get("ready_data_default_table")
    history_table = dag_infos.get("ready_data_history_table")

    # --- Extract：API 串接 ---
    api_key = Variable.get("MOENV_API_KEY")
    base_url = "https://data.moenv.gov.tw/api/v2/stat_p_45"
    limit = 1000
    offset = 0
    all_records = []

    verify_ssl_env = os.getenv("MOENV_VERIFY_SSL", "true").strip().lower()
    verify_ssl = verify_ssl_env not in {"0", "false", "no"}

    while True:
        response = requests.get(
            base_url,
            params={
                "format": "json",
                "api_key": api_key,
                "offset": offset,
                "limit": limit,
            },
            timeout=120,
            proxies=proxies,
            verify=verify_ssl,
        )
        response.raise_for_status()
        body = response.json()

        batch = []
        if isinstance(body, list):
            batch = body
        elif isinstance(body, dict):
            batch = body.get("records") or body.get("data") or []
            if not batch and isinstance(body.get("result"), dict):
                batch = body["result"].get("records") or []

        if not batch:
            break
        
        all_records.extend(batch)
        if len(batch) < limit:
            break
        offset += limit
        time.sleep(0.1)

    if not all_records:
        raise ValueError("MOENV stat_p_45 returned no records.")

    raw_data = pd.DataFrame(all_records)
    
    # --- Transform ---
    data = raw_data.copy()
    data.columns = [c.lower() for c in data.columns] # 轉小寫

    # 1. 縣市過濾 (雙北)
    target_counties = ["臺北市", "台北市", "新北市"]
    data = data[data["county"].isin(target_counties)].copy()
    
    if data.empty:
        print("⚠️ 警告：篩選後無雙北資料，結束任務。")
        return

    # 2. 民國年轉西元年 (例如 111 -> 2022)
    def convert_roc_year(y):
        try:
            return int(str(y).strip()) + 1911
        except:
            return None

    if "year" in data.columns:
        data["year"] = data["year"].apply(convert_roc_year)

    # 3. 處理數值欄位的千分位逗號並轉型
    numeric_cols = [
        "garbagegenerated", "garbageclearance", 
        "garbagerecycled", "foodwastesrecycled"
    ]
    for col in numeric_cols:
        if col in data.columns:
            data[col] = data[col].astype(str).str.replace(',', '', regex=False)
            data[col] = pd.to_numeric(data[col], errors="coerce")

    # 4. 選取最終欄位與去重
    order_cols = ["year", "county", "garbagegenerated", "garbageclearance", "garbagerecycled", "foodwastesrecycled"]
    exist_cols = [c for c in order_cols if c in data.columns]
    ready_data = data[exist_cols].dropna(subset=["year", "county"], how="any")
    
    # 以 (年度, 縣市) 為 Key 去重
    ready_data = ready_data.drop_duplicates(subset=["year", "county"], keep="last")

    # --- Load ---
    engine = create_engine(ready_data_db_uri)
    save_dataframe_to_postgresql(
        engine,
        data=ready_data,
        load_behavior=load_behavior,
        default_table=default_table,
        history_table=history_table,
    )
    print(f"✅ 已成功將 {len(ready_data)} 筆資料寫入表 {default_table}")

# --- DAG 註冊 ---
dag = CommonDag(
    proj_folder="proj_new_taipei_city_dashboard",
    dag_folder="garbage_treatment",  # 這裡要對應你的資料夾名稱
)
dag.create_dag(etl_func=_transfer)
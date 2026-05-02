import os
import time
import urllib3
import pandas as pd
import requests

from airflow import DAG
from airflow.models import Variable
from sqlalchemy import create_engine

from operators.common_pipeline import CommonDag
from utils.load_stage import save_dataframe_to_postgresql

# 關閉系統對政府網站的 SSL 不安全憑證警告
urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)


def _transfer(**kwargs):
    """
    雙北水環境大數據整合系統：Extract → Transform → Load
    資料來源：MOENV API (水庫 wqx_p_03、地下水 wqx_p_02、河川 wqx_p_01)
    客製化：動態過濾僅保留 臺北市/台北市 與 新北市 的資料
    """
    
    # --- 1. 執行環境與 DAG 目標表（由 job_config / kwargs 注入）---
    ready_data_db_uri = kwargs.get("ready_data_db_uri")
    proxies = kwargs.get("proxies")
    dag_infos = kwargs.get("dag_infos")
    load_behavior = dag_infos.get("load_behavior")
    default_table = dag_infos.get("ready_data_default_table")
    history_table = dag_infos.get("ready_data_history_table")

    # --- 2. Extract：動態設定與 API 抓取 ---
    api_key = Variable.get("MOENV_API_KEY")  # 於 Airflow 填入 MOENV API 金鑰
    base_url = "https://data.moenv.gov.tw/api/v2"
    limit = 1000
    target_counties = ["臺北市", "台北市", "新北市"]
    
    verify_ssl_env = os.getenv("MOENV_VERIFY_SSL", "true").strip().lower()
    verify_ssl = verify_ssl_env not in {"0", "false", "no"}

    # 定義三大水環境資料源
    apis = {
        "水庫水質": f"{base_url}/wqx_p_03",
        "地下水監測": f"{base_url}/wqx_p_02",
        "河川水質": f"{base_url}/wqx_p_01"  
    }

    # 全局欄位標準化對照表 (轉為底線小寫，符合 DB 習慣)
    col_mapping = {
        'sitename': 'site_name', 'damname': 'dam_name', 'river': 'river', 
        'basin': 'basin', 'ugwdistname': 'ugw_dist_name', 'itemname': 'item_name', 
        'itemvalue': 'item_value', 'itemunit': 'item_unit', 'sampledate': 'sample_date', 
        'siteid': 'site_id', 'twd97lon': 'twd97_lon', 'twd97lat': 'twd97_lat', 
        'twd97tm2x': 'twd97_tm2x', 'twd97tm2y': 'twd97_tm2y', 'township': 'township'
    }

    all_filtered_data = []

    # 針對每個 API 進行分頁抓取
    for water_type, url in apis.items():
        print(f"⏳ 開始擷取【{water_type}】資料...")
        offset = 0
        
        while True:
            response = requests.get(
                url,
                params={"format": "json", "api_key": api_key, "offset": offset, "limit": limit},
                timeout=120,
                proxies=proxies,
                verify=verify_ssl,
            )
            response.raise_for_status()
            body = response.json()

            # 解析回傳 JSON
            if isinstance(body, list):
                batch = body
            elif isinstance(body, dict):
                batch = body.get("records") or body.get("data") or []
                if not batch and isinstance(body.get("result"), dict):
                    batch = body["result"].get("records") or []
            else:
                raise TypeError(f"Unexpected JSON type: {type(body).__name__}")

            if not batch:
                break
                
            batch_df = pd.DataFrame(batch)
            
            # 統一小寫並映射標準欄位名
            batch_df.columns = batch_df.columns.str.lower()
            batch_df.columns = [col_mapping.get(c, c) for c in batch_df.columns]
            
            # 📍=========================================📍
            #   過濾邏輯：只保留「雙北」的測站 (減少記憶體負擔)
            # 📍=========================================📍
            if 'county' in batch_df.columns:
                matched_df = batch_df[batch_df['county'].isin(target_counties)].copy()
                if not matched_df.empty:
                    matched_df['water_type'] = water_type
                    all_filtered_data.append(matched_df)

            if len(batch) < limit:
                break
            
            offset += limit
            time.sleep(0.1)

    if not all_filtered_data:
        print("⚠️ 警告：本次所有 API 回傳中沒有雙北測站，提早結束任務。")
        return

    # --- 3. Transform：資料合併與清洗 ---
    df_combined = pd.concat(all_filtered_data, ignore_index=True, sort=False)
    print(f"🌍 深度挖掘完成！總計撈出雙北地區共 {len(df_combined)} 筆紀錄。")

    # 確保必要衍生欄位存在
    essential_cols = ['sample_date', 'dam_name', 'river', 'site_name', 'basin', 'ugw_dist_name', 'item_name', 'item_value', 'item_unit', 'water_type']
    for col in essential_cols:
        if col not in df_combined.columns:
            df_combined[col] = None

    # 維度塌陷與合併 (Coalescence)
    df_combined['water_body_name'] = df_combined['dam_name'].fillna(df_combined['river']).fillna(df_combined['site_name']).fillna("未知水體")
    df_combined['region_category'] = df_combined['basin'].fillna(df_combined['ugw_dist_name']).fillna("未知區域")

    # 數值清洗：移除雜訊 (<, >, N.D.) 轉為純數值
    def safe_numeric(x):
        if pd.isna(x): 
            return None
        try: 
            return float(str(x).replace('<', '').replace('>', '').replace('N.D.', '').strip())
        except: 
            return None
            
    df_combined['item_value_numeric'] = df_combined['item_value'].apply(safe_numeric)

    # 智慧時間統一清洗
    if 'sample_date' in df_combined.columns:
        df_combined['sample_date'] = df_combined['sample_date'].astype(str).str.strip()
        df_combined['sample_date'] = pd.to_datetime(df_combined['sample_date'], format='mixed', errors='coerce')
        df_combined['sample_date'] = df_combined['sample_date'].dt.strftime('%Y-%m-%d')

    # 型別強制轉換
    numeric_cols = ['item_value_numeric', 'twd97_lon', 'twd97_lat', 'twd97_tm2x', 'twd97_tm2y']
    for col in numeric_cols:
        if col in df_combined.columns:
            df_combined[col] = pd.to_numeric(df_combined[col], errors="coerce")

    # 過濾出有效數值，確保站台代碼存在
    ready_data = df_combined.dropna(subset=['item_value_numeric', 'site_id'], how='any').copy()

    # 去重：以「站台代碼 + 採樣日期 + 檢測項目」做為唯一主鍵
    before_n = len(ready_data)
    ready_data = ready_data.drop_duplicates(subset=["site_id", "sample_date", "item_name"], keep="last")
    if len(ready_data) < before_n:
        print(f"💧 去重機制啟動 (site_id, sample_date, item_name): {before_n} -> {len(ready_data)} rows")

    # 選取最終要入庫的欄位
    order_cols = [
        'site_id', 'site_name', 'dam_name', 'county', 'township',
        'twd97_lon', 'twd97_lat', 'twd97_tm2x', 'twd97_tm2y',
        'sample_date', 'item_name', 'item_value', 'item_value_numeric', 'item_unit',
        'water_type', 'water_body_name', 'region_category'
    ]
    exist = [c for c in order_cols if c in ready_data.columns]
    ready_data = ready_data[exist]

    # --- 4. Load：寫入資料庫 ---
    engine = create_engine(ready_data_db_uri)
    save_dataframe_to_postgresql(
        engine,
        data=ready_data,
        load_behavior=load_behavior,
        default_table=default_table,
        history_table=history_table,
    )


# --- DAG 註冊 ---
dag = CommonDag(
    proj_folder="proj_new_taipei_city_dashboard",
    dag_folder="a_water_quality_integrated",  # 👈 將原本的 b_ 改成 a_
)
dag.create_dag(etl_func=_transfer)
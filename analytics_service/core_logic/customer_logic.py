import datetime
from sqlalchemy.orm import Session
from decimal import Decimal

from repositories import customer_repo, etl_repo

from core_logic.utils import to_string
from core_logic.cache_manager import get_cache, set_cache, delete_cache

CACHE_KEY = 900

def get_customer_analytics_logic(
  db: Session,
  date_range: object,
  product_group_id: int | None,
  pagination: object
) -> dict:
  try:
    start_date = datetime.date.fromisoformat(date_range.start_date)
    end_date = datetime.date.fromisoformat(date_range.end_date)
  except ValueError:
    raise ValueError("Invalid date format. Use YYYY-MM-DD.")
  
  page_size = pagination.page_size if pagination.page_size > 0 else 20

  last_id = 0
  if pagination.page_token:
    try:
      last_id = int(pagination.page_token)
    except ValueError:
      raise ValueError("Invalid page token.")
    
  p_group_id = product_group_id if product_group_id is not None else "all"  
  count_cache_key = (
    f"reports:customer-analytics:count:"
    f"start={date_range.start_date}:end={date_range.end_date}:"
    f"group={p_group_id}"
  )

  total_count = get_cache(count_cache_key)

  if total_count is None:
    total_count = customer_repo.count_total_customer_analytics(
      db, start_date, end_date, product_group_id
    )
    set_cache(
      count_cache_key,
      total_count,
      CACHE_KEY
    )
  else:
    print("CACHE HIT")
    total_count = int(total_count)

  raw_analytics = customer_repo.get_customer_analytics_paginated(
    db, start_date, end_date, product_group_id, page_size, last_id
  )

  formatted_analytics = []
  for item in raw_analytics:
    formatted_analytics.append({
      "id": item['id'],
      "date": item['date'].isoformat(),
      "product_group_id": item['product_group_id'],
      "total_transactions": item['total_transactions'],
      "total_revenue": to_string(item['total_revenue']),
      "average_transaction_value": to_string(item['average_transaction_value']),
      # "peak_hour": item['peak_hour'],
      "created_at": item['created_at'],
      "updated_at": item['updated_at']
    })
  
  next_page_token = ""
  if formatted_analytics:
    last_item_id = formatted_analytics[-1]['id']
    next_page_token = str(last_item_id)
  
  return {
    "data": formatted_analytics,
    "total_count": total_count,
    "next_page_token": next_page_token,
  }

def get_weekly_peak_hours_logic(db: Session) -> list[dict]:
    """
    Mengambil data pola jam sibuk mingguan, diformat untuk respons gRPC baru.
    """
    cache_key = "reports:weekly-peak-hours:v1" # Kunci cache statis baru

    cached_data = get_cache(cache_key)
    if cached_data:
        print("CACHE HIT")
        return cached_data
    
    # 1. Inisialisasi struktur data lengkap
    # Ini memastikan Anda mengembalikan 24 jam untuk setiap hari,
    # bahkan jika tidak ada penjualan (transaction_count: 0)
    weekly_map = {}
    for day in range(1, 8): 
        weekly_map[day] = {
            hour: {"transaction_count": 0, "total_revenue": Decimal(0)}
            for hour in range(24) # 0 (00:00) sampai 23 (23:00)
        }

    # 2. Ambil data mentah dari database
    try:
        raw_data = customer_repo.get_weekly_peak_hour_data(db)
    except Exception as e:
        print(f"Error querying database: {e}")
        raise e

    # 3. Isi struktur data dengan data dari DB
    for row in raw_data:
        day = int(row['day_of_week'])
        hour = int(row['hour_of_day'])
        
        # Pastikan data ada di dalam rentang yang diharapkan
        if day in weekly_map and hour in weekly_map[day]:
            weekly_map[day][hour] = {
                "transaction_count": int(row['transaction_count']),
                "total_revenue": row['total_revenue'] or Decimal(0)
            }

    # 4. Ubah format map menjadi daftar yang diminta oleh .proto
    response_list = []
    for day_of_week, hourly_map in weekly_map.items():
        day_data = {
            "day_of_week": day_of_week,
            "hourly_data": []
        }
        
        # Urutkan berdasarkan jam
        for hour in sorted(hourly_map.keys()):
            data = hourly_map[hour]
            day_data["hourly_data"].append({
                "hour": f"{hour:02d}:00", # Format jam: "09:00"
                "transaction_count": data['transaction_count'],
                "total_revenue": to_string(data['total_revenue'])
            })
            
        response_list.append(day_data)

    # 5. Simpan ke cache
    set_cache(cache_key, response_list, CACHE_KEY) # Menggunakan TTL 900 detik
  
    return response_list

def generate_customer_analytics_logic(
    analytics_db: Session,
    date_str: str,
    product_group_id: int | None
) -> list[dict]:
    try:
        date = datetime.date.fromisoformat(date_str)
    except ValueError:
        raise ValueError("Format tanggal salah. Gunakan YYYY-MM-DD.")
        
    # 1. EXTRACT & TRANSFORM
    raw_customer_data = etl_repo.get_raw_customer_analytics_data(
        analytics_db, date, product_group_id
    )
    
    if not raw_customer_data:
        print(f"[Logic] Tidak ada data customer analytics untuk tanggal {date_str}.")
        return []
        
    processed_data = []
    try:
        # 2. TRANSFORM (Hitung Rata-rata)
        for row in raw_customer_data:
            total_revenue = Decimal(row['total_revenue'])
            total_transactions = int(row['total_transactions'])
            
            avg_value = Decimal(0)
            if total_transactions > 0:
                avg_value = total_revenue / Decimal(total_transactions)
            
            row_to_upsert = {
                "date": row['date'],
                "product_group_id": row['product_group_id'],
                "total_transactions": total_transactions,
                "total_revenue": total_revenue,
                "average_transaction_value": avg_value
            }
            
            # 3. LOAD (Upsert ke DB)
            etl_repo.upsert_customer_analytics(analytics_db, row_to_upsert)
            processed_data.append(row_to_upsert) # Simpan untuk respons
        
        analytics_db.commit() # Commit semua perubahan sekaligus
        
    except Exception as e:
        analytics_db.rollback()
        print(f"Error saat upsert data customer analytics: {e}")
        raise e

    # 4. HAPUS CACHE (PENTING)
    cache_pattern = f"reports:customer-analytics:count:start={date_str}*"
    delete_cache(cache_pattern)
    
    # 5. Kembalikan data yang baru saja diproses
    return processed_data
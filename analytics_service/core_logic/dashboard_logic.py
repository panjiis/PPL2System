import datetime
import json
from sqlalchemy.orm import Session
from decimal import Decimal

from repositories import dashboard_repo
from core_logic.utils import to_string, to_percent_string
from core_logic.cache_manager import get_cache, set_cache, redis_client

CACHE_TTL = 900

LOW_STOCK_HASH_KEY = "dashboard:low_stock_hash"
PENDING_COMMISSIONS_KEY = "dashboard:pending_commissions"

def _get_dashboard_data_from_db(
  analytics_db: Session,
  # inventory_db: Session,
  # commissions_db: Session,
  date_str: str
) -> dict:
  try:
    date_today = datetime.date.fromisoformat(date_str)
    date_yesterday = date_today - datetime.timedelta(days=1)
  except ValueError:
    raise ValueError("Incorrect date format. Use YYYY-MM-DD.")
  
  kpi_today_raw = dashboard_repo.get_kpi_for_date(analytics_db, date_today)
  kpi_today = {
    'revenue': kpi_today_raw.get('total_revenue', Decimal(0)),
    'transactions': kpi_today_raw.get('total_transactions', 0),
    'items_sold': kpi_today_raw.get('total_items_sold', 0),
    'profit': kpi_today_raw.get('total_gross_profit', Decimal(0))
  }

  kpi_yesterday_raw = dashboard_repo.get_kpi_for_date(analytics_db, date_yesterday)
  kpi_yesterday = {
    'revenue': kpi_yesterday_raw.get('total_revenue', Decimal(0)),
    'transactions': kpi_yesterday_raw.get('total_transactions', 0),
  }

  if kpi_yesterday['revenue'] > 0:
    revenue_change = ((kpi_today['revenue'] - kpi_yesterday['revenue']) / kpi_yesterday['revenue']) * 100
  else:
    revenue_change = Decimal(100) if kpi_today['revenue'] > 0 else Decimal(0)
  
  if kpi_yesterday['transactions'] > 0:
    tx_change = ((Decimal(kpi_today['transactions']) - Decimal(kpi_yesterday['transactions'])) / Decimal(kpi_yesterday['transactions'])) * Decimal(100)
  else:
    tx_change = Decimal(100) if kpi_today['transactions'] > 0 else Decimal(0)
  
  top_products = dashboard_repo.get_top_products_for_date(analytics_db, date_today, limit=5)
  top_performers = dashboard_repo.get_top_performers_for_date(analytics_db, date_today, limit=5)

  # low_stock_alerts = dashboard_repo.get_low_stock_alerts(inventory_db)

  low_stock_alerts = []
  if redis_client:
      try:
          # Ambil semua data dari HASH (hasilnya: {'Ayam Rica': '4', 'Produk B': '2'})
          stock_hash = redis_client.hgetall(LOW_STOCK_HASH_KEY) 
          
          # Ubah HASH menjadi daftar (list) dictionary
          for product_name, remaining_quantity in stock_hash.items():
            low_stock_alerts.append({
                "product_name": product_name,
                "remaining_quantity": int(remaining_quantity)
            })
          print(f"low stock:\n {low_stock_alerts}")
      except Exception as e:
          print(f"Gagal mengambil low_stock_alerts dari Redis HASH: {e}")
  else:
      print("Redis client tidak tersedia untuk low stock.")

  # pending_commissions = dashboard_repo.get_pending_commissions_count(commissions_db)

  pending_commissions = 0
  if redis_client:
      try:
          # Ambil nilai dari penghitung (counter) Redis
          pending_commissions = int(redis_client.get(PENDING_COMMISSIONS_KEY) or 0)
      except Exception as e:
          print(f"Gagal mengambil pending_commissions_count dari Redis: {e}")
  else:
      print("Redis client tidak tersedia untuk pending commissions.")

  dashboard_data = {
    "today_revenue": to_string(kpi_today['revenue']),
    "today_transactions": int(kpi_today['transactions']),
    "today_items_sold": int(kpi_today['items_sold']),
    "today_profit": to_string(kpi_today['profit']),
    
    "revenue_change_percentage": to_percent_string(revenue_change),
    "transaction_change_percentage": to_percent_string(tx_change),
    
    "top_products_today": top_products,
    "top_performers_today": top_performers, 
    
    "low_stock_alerts": low_stock_alerts,
    "pending_commissions_count": pending_commissions
  }

  return dashboard_data

def get_dashboard_data_logic(
  analytics_db: Session,
  # inventory_db: Session,
  # commissions_db: Session,
  date_str: str
) -> dict:
  cache_key = f"dashboard:main:{date_str}"

  cached_data = get_cache(cache_key)

  if cached_data:
    print("CACHE HIT")
    return cached_data

  try:
    db_data = _get_dashboard_data_from_db(
      analytics_db, date_str
    )

  except Exception as e:
    print(f"Error querying database: {e}")
    raise e
  
  if db_data:
    set_cache(
      cache_key, db_data, CACHE_TTL
    )
  
  return db_data

# def get_real_time_metrics_logic(db: Session) -> dict:
#   LARGE_TRANSACTION_THRESHOLD = Decimal("500000.00")
#   RECENT_TRANSACTION_LIMIT = 5

#   metrics = dashboard_repo.get_realtime_metrics_from_pos(db)
#   active_tx_count = dashboard_repo.get_active_transactions_count(db)
#   large_tx_raw = dashboard_repo.get_recent_large_transactions(
#     db, LARGE_TRANSACTION_THRESHOLD, RECENT_TRANSACTION_LIMIT
#   )

#   hourly_revenue = metrics.get('hourly_revenue', Decimal(0)) if metrics.get('hourly_revenue') else Decimal(0)
#   hourly_tx_count = metrics.get('hourly_transaction_count', 0) if metrics.get('hourly_transaction_count') else 0

#   avg_tx_value = (
#     (hourly_revenue / hourly_tx_count) if hourly_tx_count > 0 else Decimal(0)
#   )

#   recent_large_transactions = [
#     f"ID: {tx['id']} - Amount: {str(tx['total_amount'])}" for tx in large_tx_raw
#   ]

#   real_time_data = {
#     "last_updated": datetime.datetime.now(datetime.timezone.utc),
#     "active_transactions": int(active_tx_count),
#     "hourly_revenue": str(hourly_revenue),
#     "hourly_transaction_count": int(hourly_tx_count),
#     "average_transaction_value": str(avg_tx_value),
#     "recent_large_transactions": recent_large_transactions
#   }

#   return real_time_data

def get_realtime_metrics_from_cache() -> dict:
    
    # 1. Dapatkan kunci dan jam saat ini
    now = datetime.datetime.now()
    today_key = now.strftime("metrics:%Y-%m-%d") # Kunci per hari
    current_hour_str = str(now.hour) # Jam saat ini

    # 2. Ambil semua data dari Redis sekaligus
    pipe = redis_client.pipeline()
    pipe.hgetall(today_key) # Mendapat semua data harian (total dan per jam)
    pipe.lrange("metrics:realtime:large_tx", 0, 4)
    pipe.get("metrics:realtime:last_updated")
    results = pipe.execute()
    
    daily_data = results[0]
    large_tx_json = results[1]
    last_updated = results[2]

    # 3. Hitung Rata-rata (Asumsi 2: Rata-rata harian)
    total_rev = Decimal(daily_data.get("total_revenue", 0))
    total_tx = int(daily_data.get("total_transactions", 0))
    avg_tx_value = (total_rev / total_tx) if total_tx > 0 else Decimal(0)
    
    # 4. Format transaksi besar
    large_tx_list = []
    for tx_string in large_tx_json:
        try:
            large_tx_list.append(json.loads(tx_string))
        except json.JSONDecodeError:
            print(f"Error: Gagal mem-parsing JSON dari Redis: {tx_string}")
    
    # 5. Format hasil
    return {
        "last_updated": last_updated or now.isoformat(),
        # "active_transactions": 0, # Dihapus
        "hourly_revenue": daily_data.get(f"hour_revenue:{current_hour_str}", "0.00"),
        "hourly_transaction_count": int(daily_data.get(f"hour_count:{current_hour_str}", 0)),
        "average_transaction_value": to_string(avg_tx_value), # Gunakan util 'to_string' Anda
        "recent_large_transactions": large_tx_list 
    }

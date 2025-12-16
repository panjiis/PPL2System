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

# core_logic/dashboard_logic.py

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

def _get_realtime_operational_data() -> dict:
    """
    Fungsi Helper: Mengambil data operasional langsung dari Redis.
    Data ini sifatnya real-time dan TIDAK BOLEH di-cache lama-lama.
    """
    low_stock_alerts = []
    pending_commissions = 0

    if redis_client:
        # 1. Ambil Low Stock Alerts
        try:
            stock_hash = redis_client.hgetall(LOW_STOCK_HASH_KEY)
            for product_name, remaining_quantity in stock_hash.items():
                low_stock_alerts.append({
                    "product_name": product_name,
                    "remaining_quantity": int(remaining_quantity)
                })
        except Exception as e:
            print(f"Redis Error (Low Stock): {e}")

        # 2. Ambil Pending Commissions
        try:
            pending_commissions = int(redis_client.get(PENDING_COMMISSIONS_KEY) or 0)
        except Exception as e:
            print(f"Redis Error (Pending Comm): {e}")
    else:
        print("Redis client tidak tersedia untuk data realtime.")

    return {
        "low_stock_alerts": low_stock_alerts,
        "pending_commissions_count": pending_commissions
    }

def _get_dashboard_data_from_db(
    analytics_db: Session,
    date_str: str
) -> dict:
    """
    Mengambil data analitik berat (KPI, Charts) dari Database SQL.
    Fungsi ini output-nya AMAN untuk di-cache selama 15 menit.
    """
    try:
        date_today = datetime.date.fromisoformat(date_str)
        date_yesterday = date_today - datetime.timedelta(days=1)
    except ValueError:
        raise ValueError("Incorrect date format. Use YYYY-MM-DD.")
    
    # --- KPI HARI INI ---
    kpi_today_raw = dashboard_repo.get_kpi_for_date(analytics_db, date_today)
    kpi_today = {
        'revenue': kpi_today_raw.get('total_revenue', Decimal(0)),
        'transactions': kpi_today_raw.get('total_transactions', 0),
        'items_sold': kpi_today_raw.get('total_items_sold', 0),
        'profit': kpi_today_raw.get('total_gross_profit', Decimal(0))
    }

    # --- KPI KEMARIN (Untuk Persentase) ---
    kpi_yesterday_raw = dashboard_repo.get_kpi_for_date(analytics_db, date_yesterday)
    kpi_yesterday = {
        'revenue': kpi_yesterday_raw.get('total_revenue', Decimal(0)),
        'transactions': kpi_yesterday_raw.get('total_transactions', 0),
    }

    # Hitung Persentase Revenue
    if kpi_yesterday['revenue'] > 0:
        revenue_change = ((kpi_today['revenue'] - kpi_yesterday['revenue']) / kpi_yesterday['revenue']) * 100
    else:
        revenue_change = Decimal(100) if kpi_today['revenue'] > 0 else Decimal(0)
    
    # Hitung Persentase Transaksi
    if kpi_yesterday['transactions'] > 0:
        tx_change = ((Decimal(kpi_today['transactions']) - Decimal(kpi_yesterday['transactions'])) / Decimal(kpi_yesterday['transactions'])) * Decimal(100)
    else:
        tx_change = Decimal(100) if kpi_today['transactions'] > 0 else Decimal(0)
    
    # --- TOP CHARTS ---
    top_products = dashboard_repo.get_top_products_for_date(analytics_db, date_today, limit=1)
    top_performers = dashboard_repo.get_top_performers_for_date(analytics_db, date_today, limit=5)

    # Catatan: Kita HAPUS pengambilan Redis dari sini agar tidak ikut ter-cache statis.
    # Kita berikan nilai kosong dulu, nanti diisi di fungsi utama.

    dashboard_data = {
        "today_revenue": to_string(kpi_today['revenue']),
        "today_transactions": int(kpi_today['transactions']),
        "today_items_sold": int(kpi_today['items_sold']),
        "today_profit": to_string(kpi_today['profit']),
        
        "revenue_change_percentage": to_percent_string(revenue_change),
        "transaction_change_percentage": to_percent_string(tx_change),
        
        "top_products_today": top_products,
        "top_performers_today": top_performers, 
        
        # Placeholder (akan ditimpa dengan data realtime)
        "low_stock_alerts": [],
        "pending_commissions_count": 0
    }

    return dashboard_data

def get_dashboard_data_logic(
    analytics_db: Session,
    date_str: str
) -> dict:
    """
    Logika Utama: Menggabungkan Data Cached (DB) + Data Realtime (Redis).
    """
    cache_key = f"dashboard:main:{date_str}"

    # 1. AMBIL DATA STATIS (KPI, Charts) - Coba dari Cache dulu
    db_data = get_cache(cache_key)

    if not db_data:
        # Cache Miss: Ambil dari DB
        try:
            db_data = _get_dashboard_data_from_db(analytics_db, date_str)
            # Simpan ke Cache (TTL 15 menit)
            set_cache(cache_key, db_data, CACHE_TTL)
        except Exception as e:
            print(f"Error querying database: {e}")
            raise e
    else:
        print("CACHE HIT (SQL Data)")

    # 2. AMBIL DATA REALTIME (Low Stock, Alerts) - Selalu Fresh dari Redis
    realtime_data = _get_realtime_operational_data()

    # 3. MERGE (GABUNGKAN)
    # Timpa placeholder dengan data realtime terbaru
    db_data["low_stock_alerts"] = realtime_data["low_stock_alerts"]
    db_data["pending_commissions_count"] = realtime_data["pending_commissions_count"]
    
    # Print debug untuk memastikan
    # print(f"DEBUG Realtime Data: {realtime_data}")

    return db_data

def get_realtime_metrics_from_cache() -> dict:
    """
    Mengambil metrik grafik per jam (Hourly Revenue) dari Redis.
    Fungsi ini tidak berubah dari sebelumnya.
    """
    
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

    # 3. Hitung Rata-rata
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
        "hourly_revenue": daily_data.get(f"hour_revenue:{current_hour_str}", "0.00"),
        "hourly_transaction_count": int(daily_data.get(f"hour_count:{current_hour_str}", 0)),
        "average_transaction_value": to_string(avg_tx_value),
        "recent_large_transactions": large_tx_list 
    }

def clear_low_stock_cache_logic() -> bool:
    if redis_client:
        try:
            # Menghapus key HASH low stock
            redis_client.delete(LOW_STOCK_HASH_KEY)
            print(f"🗑️ Cache Low Stock deleted: {LOW_STOCK_HASH_KEY}")
            return True
        except Exception as e:
            print(f"Error clearing low stock cache: {e}")
            return False
    return False



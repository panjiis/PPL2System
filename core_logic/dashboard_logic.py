import datetime
from sqlalchemy.orm import Session
from decimal import Decimal

from repositories import dashboard_repo
from core_logic.utils import to_string, to_percent_string
from core_logic.cache_manager import get_cache, set_cache

CACHE_TTL = 900
CACHE_TTL_REALTIME = 3

def _get_dashboard_data_from_db(
  analytics_db: Session,
  inventory_db: Session,
  commissions_db: Session,
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

  low_stock_alerts = dashboard_repo.get_low_stock_alerts(inventory_db)

  pending_commissions = dashboard_repo.get_pending_commissions_count(commissions_db)

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
  inventory_db: Session,
  commissions_db: Session,
  date_str: str
) -> dict:
  cache_key = f"dashboard:main:{date_str}"

  cached_data = get_cache(cache_key)

  if cached_data:
    print("CACHE HIT")
    return cached_data

  try:
    db_data = _get_dashboard_data_from_db(
      analytics_db, inventory_db, commissions_db, date_str
    )

  except Exception as e:
    print(f"Error querying database: {e}")
    raise e
  
  if db_data:
    set_cache(
      cache_key, db_data, CACHE_TTL
    )
  
  return db_data

def get_real_time_metrics_logic(db: Session) -> dict:
  LARGE_TRANSACTION_THRESHOLD = Decimal("500000.00")
  RECENT_TRANSACTION_LIMIT = 5

  metrics = dashboard_repo.get_realtime_metrics_from_pos(db)
  active_tx_count = dashboard_repo.get_active_transactions_count(db)
  large_tx_raw = dashboard_repo.get_recent_large_transactions(
    db, LARGE_TRANSACTION_THRESHOLD, RECENT_TRANSACTION_LIMIT
  )

  hourly_revenue = metrics.get('hourly_revenue', Decimal(0)) if metrics.get('hourly_revenue') else Decimal(0)
  hourly_tx_count = metrics.get('hourly_transaction_count', 0) if metrics.get('hourly_transaction_count') else 0

  avg_tx_value = (
    (hourly_revenue / hourly_tx_count) if hourly_tx_count > 0 else Decimal(0)
  )

  recent_large_transactions = [
    f"ID: {tx['id']} - Amount: {to_string(tx['total_amount'])}" for tx in large_tx_raw
  ]

  real_time_data = {
    "last_updated": datetime.datetime.now(datetime.timezone.utc),
    "active_transactions": int(active_tx_count),
    "hourly_revenue": to_string(hourly_revenue),
    "hourly_transaction_count": int(hourly_tx_count),
    "average_transaction_value": to_string(avg_tx_value),
    "recent_large_transactions": recent_large_transactions
  }

  return real_time_data

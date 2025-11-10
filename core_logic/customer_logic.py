import datetime
from sqlalchemy.orm import Session
from decimal import Decimal

from repositories import customer_repo

from core_logic.utils import to_string
from core_logic.cache_manager import get_cache, set_cache

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
      "peak_hour": item['peak_hour'],
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

def _get_peak_hours_logic_from_db(
  db: Session,
  date_range: object,
) -> list[dict]:
  try:
    start_date = datetime.date.fromisoformat(date_range.start_date)
    end_date = datetime.date.fromisoformat(date_range.end_date)
  except ValueError:
    raise ValueError("Invalid date format. Use YYYY-MM-DD.")
  
  # raw_data = customer_repo.get_peak_hour_data_from_pos(
  #   db, start_date, end_date
  # )

  raw_data = customer_repo.get_peak_hour_data(
    db, start_date, end_date
  )

  hourly_map = {
    hour: {
      "transaction_count": 0,
      "total_revenue": Decimal(0)
    } for hour in range(24)
  }

  for item in raw_data:
    hour = int(item['hour_of_day'])
    if hour in hourly_map:
      hourly_map[hour] = {
        "transaction_count": item['transaction_count'],
        "total_revenue": item['total_revenue']
      }
  
  formatted_data = []
  for hour, data in hourly_map.items():
    formatted_data.append({
      "hour": f"{hour:02d}:00",
      "transaction_count": data['transaction_count'],
      "total_revenue": to_string(data['total_revenue'])
    })
  
  return formatted_data

def get_peak_hours_logic(
  db: Session,
  date_range: object,
) -> list[dict]:  
  cache_key = (
    f"reports:peak-hours:" # Key prefix baru
    f"start={date_range.start_date}:end={date_range.end_date}"
  )

  cached_data = get_cache(cache_key)

  if cached_data:
    print("CACHE HIT")
    return cached_data
  
  try:
    db_data = _get_peak_hours_logic_from_db(
      db, date_range
    )
  except Exception as e:
    print(f"Error querying database: {e}")
    raise e

  if db_data:
    set_cache(
      cache_key, db_data, CACHE_KEY
    )
  
  return db_data
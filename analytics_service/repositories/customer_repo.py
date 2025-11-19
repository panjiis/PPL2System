import datetime
from sqlalchemy.orm import Session
from sqlalchemy import text

def get_customer_analytics_paginated(
  db: Session,
  start_date: datetime.date,
  end_date: datetime.date,
  product_group_id: int | None,
  page_size: int,
  last_id: int
) -> list[dict]:
  params = {
    "start_date": start_date,
    "end_date": end_date,
    "page_size": page_size,
    "last_id": last_id
  }

  query_str = """
    SELECT * FROM customer_analytics
    WHERE date BETWEEN :start_date AND :end_date
      AND id > :last_id
  """

  if product_group_id:
    query_str += " AND product_group_id = :product_group_id"
    params["product_group_id"] = product_group_id

  query_str += " ORDER BY id ASC LIMIT :page_size"

  result = db.execute(text(query_str), params).all()

  analytics_data =[]
  for row in result:
    analytics_data.append({
      'id': row.id,
      'date': row.date,
      'product_group_id': row.product_group_id,
      'total_transactions': row.total_transactions,
      'total_revenue': row.total_revenue,
      'average_transaction_value': row.average_transaction_value,
      # 'peak_hour': row.peak_hour,
      'created_at': row.created_at,
      'updated_at': row.updated_at
    })
  return analytics_data

def count_total_customer_analytics(
  db: Session,
  start_date: datetime.date,
  end_date: datetime.date,
  product_group_id: int | None
) -> int:
  params = {
    "start_date": start_date,
    "end_date": end_date
  }

  query_str = """
    SELECT COUNT(*) FROM customer_analytics
    WHERE date BETWEEN :start_date AND :end_date
  """

  if product_group_id:
    query_str += " AND product_group_id = :product_group_id"
    params["product_group_id"] = product_group_id
  
  result = db.execute(text(query_str), params).scalar_one_or_none()

  return result or 0

# def get_peak_hour_data( # from analytics  
#   db: Session,
#   start_date: datetime.date,
#   end_date: datetime.date
# ): 
#   start_dt = datetime.datetime.combine(start_date, datetime.time.min)
#   end_dt = datetime.datetime.combine(end_date, datetime.time.max)

#   params = {
#     "start_dt": start_dt,
#     "end_dt": end_dt
#   }

#   query_str = """
#     SELECT
#       EXTRACT(HOUR FROM order_timestamp) as hour_of_day,
#       COUNT(id) as transaction_count,
#       SUM(total_amount) as total_revenue
#     FROM raw_sales_events
#     WHERE order_timestamp BETWEEN :start_dt AND :end_dt
#     GROUP BY hour_of_day
#     ORDER BY hour_of_day ASC
#   """

#   result = db.execute(text(query_str), params).all()
#   return [dict(row._mapping) for row in result]

def get_weekly_peak_hour_data(db: Session) -> list[dict]:
    query_str = """
        SELECT
            EXTRACT(DOW FROM order_timestamp) + 1 as day_of_week,
            EXTRACT(HOUR FROM order_timestamp) as hour_of_day,
            COUNT(id) as transaction_count,
            SUM(total_amount) as total_revenue
        FROM raw_sales_events
        -- Anda bisa menambahkan filter WHERE di sini jika Anda hanya ingin menganalisis data 90 hari terakhir
        -- WHERE order_timestamp >= (NOW() - INTERVAL '90 days')
        GROUP BY day_of_week, hour_of_day
        ORDER BY day_of_week, hour_of_day;
    """

    result = db.execute(text(query_str)).all()
    return [dict(row._mapping) for row in result]
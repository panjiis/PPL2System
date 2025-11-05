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
      'peak_hour': row.peak_hour,
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

def get_peak_hour_data_from_pos(
  db: Session,
  start_date: datetime.date,
  end_date: datetime.date
) -> list[dict]:
  start_dt = datetime.datetime.combine(start_date, datetime.time.min)
  end_dt = datetime.datetime.combine(end_date, datetime.time.max)

  params = {
    "start_dt": start_dt,
    "end_dt": end_dt
  }

  query_str = """
    SELECT
      EXTRACT(HOUR FROM orders_date) as hour_of_day,
      COUNT(id) as transaction_count,
      SUM(CAST(total_amount AS decimal)) as total_revenue
    FROM order_documents
    WHERE orders_date BETWEEN :start_dt AND :end_dt
    GROUP BY hour_of_day
    ORDER BY hour_of_day ASC
  """

  result = db.execute(text(query_str), params).all()
  return [dict(row._mapping) for row in result]
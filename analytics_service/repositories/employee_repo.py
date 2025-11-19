import datetime
from sqlalchemy.orm import Session
from sqlalchemy import text

CACHE_TTL = 900

def get_employee_performance_paginated(
  db: Session,
  start_date: datetime.date,
  end_date: datetime.date,
  employee_id: int | None,
  page_size: int,
  last_id: int
) -> list[dict]:
  params = {
  "start_date": start_date,
  "end_date": end_date,
  "last_id": last_id,
  "page_size": page_size
  }

  query_str = """
      SELECT * FROM employee_performance
      WHERE 
          period_start <= :end_date AND period_end >= :start_date
          AND id > :last_id
  """

  if employee_id:
    query_str += " AND employee_id = :employee_id"
    params["employee_id"] = employee_id
  
  query_str += " ORDER BY id ASC LIMIT :page_size"

  result = db.execute(text(query_str), params).all()
  return [dict(row._mapping) for row in result]

def count_total_employee_performance(
  db: Session,
  start_date: datetime.date,
  end_date: datetime.date,
  employee_id: int | None
) -> int:
  params = {
    "start_date": start_date,
    "end_date": end_date
  }

  query_str = """
        SELECT COUNT(*) FROM employee_performance
        WHERE 
            period_start <= :end_date AND period_end >= :start_date
    """

  if employee_id:
    query_str += " AND employee_id = :employee_id"
    params["employee_id"] = employee_id

  result = db.execute(text(query_str), params).scalar_one_or_none()
  return result or 0

def get_employee_performance_by_date(
    db: Session,
    start_date: datetime.date,
    end_date: datetime.date,
    employee_id: int | None
) -> list[dict]:
  params = {
    "start_date": start_date,
    "end_date": end_date
  }

  query_str = """
    SELECT * FROM employee_performance
    WHERE date BETWEEN :start_date AND :end_date
  """

  if employee_id:
    query_str += " AND employee_id = :employee_id"
    params["employee_id"] = employee_id
  
  query_str += " ORDER BY date, employee_id"

  result = db.execute(text(query_str), params).all()

  performances = []
  for row in result:
    performances.append({
      'id': row.id,
      'date': row.date,
      'employee_id': row.employee_id,
      'total_sales': row.total_sales,
      'total_transactions': row.total_transactions,
      'total_items_sold': row.total_items_sold,
      'commission_earned': row.commission_earned,
      'performance_score': row.performance_score,
      'created_at': row.created_at,
      'updated_at': row.updated_at
    })
  return performances

def get_company_top_performer(
    db: Session,
    start_date: datetime.date,
    end_date: datetime.date
) -> dict:
  params = {
    "start_date": start_date,
    "end_date": end_date
  }

  query_str = """
    SELECT
      employee_id,
      SUM(total_sales) as total_sales
    FROM employee_performance
    WHERE date BETWEEN :start_date AND :end_date
    GROUP BY employee_id
    ORDER BY total_sales DESC
    LIMIT 1
  """

  result = db.execute(text(query_str), params).first()
  return dict(result._mapping) if result else {}

def get_performance_report_data(
    db: Session,
    start_date: datetime.date,
    end_date: datetime.date,
    employee_id: int | None
) -> list[dict]:
    params = {
        "start_date": start_date,
        "end_date": end_date
    }
    
    query_str = """
        SELECT * FROM employee_performance
        WHERE 
            period_start <= :end_date AND period_end >= :start_date
    """

    if employee_id:
        query_str += " AND employee_id = :employee_id"
        params["employee_id"] = employee_id

    query_str += " ORDER BY total_sales DESC" 
    
    result = db.execute(text(query_str), params).all()
    return [dict(row._mapping) for row in result]

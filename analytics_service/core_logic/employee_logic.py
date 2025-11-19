import datetime
from sqlalchemy.orm import Session
from decimal import Decimal

from repositories import employee_repo, etl_repo
from repositories.models import RawFinalizedCommission

from core_logic.utils import to_string
from core_logic.cache_manager import set_cache, get_cache

CACHE_TTL = 900

def get_employee_performance_logic(
  db: Session,
  date_range: object,
  employee_id: int | None,
  pagination: object
) -> dict:
  try:
    start_date = datetime.date.fromisoformat(date_range.start_date)
    end_date = datetime.date.fromisoformat(date_range.end_date)
  except ValueError:
    raise ValueError("Incorrect date format in date_range. Use YYYY-MM-DD.")
  
  page_size = pagination.page_size if pagination.page_size > 0 else 20

  last_id = 0
  if pagination.page_token:
    try:
      last_id = int(pagination.page_token)
    except ValueError:
      raise ValueError(f"Invalid page_token: {pagination.page_token}")
  
  p_employee_id = employee_id if employee_id is not None else "all"

  count_cache_key = (
    f"reports:employee-perf:count:"
    f"start={date_range.start_date}:end={date_range.end_date}:"
    f"emp={p_employee_id}"
  )

  total_count = get_cache(count_cache_key)

  if total_count is None:
    total_count = employee_repo.count_total_employee_performance(
      db, start_date, end_date, employee_id
    )
    set_cache(
      count_cache_key,
      total_count,
      CACHE_TTL
    )
  else:
    print("CACHE HIT")
    total_count = int(total_count)

  raw_performances = employee_repo.get_employee_performance_paginated(
    db, start_date, end_date, employee_id, page_size, last_id
  )

  formatted_performances = []
  for item in raw_performances:
    formatted_performances.append({
      "id": item['id'],
      # "date": item['date'].isoformat(),
      "calculation_id": item['calculation_id'],
      "employee_id": item['employee_id'],
      "period_start": item['period_start'], 
      "period_end": item['period_end'],
      "total_sales": to_string(item['total_sales']),
      "total_transactions": item['total_transactions'],
      "total_items_sold": item['total_items_sold'],
      "commission_earned": to_string(item['commission_earned']),
      # "performance_score": to_string(item['performance_score']),
      "created_at": item['created_at'],
      "updated_at": item['updated_at']
    })
  
  next_page_token = ""
  if formatted_performances:
    last_item_id = formatted_performances[-1]['id']
    next_page_token = str(last_item_id)
  
  return {
    "data": formatted_performances,
    "total_count": total_count,
    "next_page_token": next_page_token
  }

def _get_performance_report_from_db(
  db: Session,
  date_range: object,
  employee_id: int | None
) -> dict:
  try:
    start_date = datetime.date.fromisoformat(date_range.start_date)
    end_date = datetime.date.fromisoformat(date_range.end_date)
  except ValueError:
    raise ValueError("Incorrect date format in date_range. Use YYYY-MM-DD.")
  
  raw_performances = employee_repo.get_performance_report_data(
    db, start_date, end_date, employee_id
  )
  # raw_performances = employee_repo.get_employee_performance_by_date(
  #   db, start_date, end_date, employee_id
  # )

  # top_performer_data = employee_repo.get_company_top_performer(
  #   db, start_date, end_date
  # )

  total_commissions = Decimal(0)
  top_performer_id = 0
  top_performer_sales = Decimal(0)
  
  formatted_performances = []
  
  if not raw_performances:
    return {
        "period": {"start_date": start_date.isoformat(), "end_date": end_date.isoformat()},
        "total_commissions": "0.00",
        "top_performer_employee_id": 0,
        "top_performer_sales": "0.00",
        "employee_performances": []
    }
  
  top_performer_id = raw_performances[0]['employee_id']
  top_performer_sales = Decimal(raw_performances[0]['total_sales'])

  for item in raw_performances:
    commission = Decimal(item['commission_earned'])
    total_commissions += commission

    formatted_performances.append({
      "id": item['id'],
      "calculation_id": item['calculation_id'],
      "employee_id": item['employee_id'],
      "period_start": item['period_start'],
      "period_end": item['period_end'],
      "total_sales": to_string(item['total_sales']),
      "total_transactions": item['total_transactions'],
      "total_items_sold": item['total_items_sold'],
      "commission_earned": to_string(item['commission_earned']),
      # "performance_score": to_string(item['performance_score']),
      "created_at": item['created_at'],
      "updated_at": item['updated_at']
    })
  
  report_data = {
    "period": {
      "start_date": date_range.start_date,
      "end_date": date_range.end_date
    },
    "employee_performances": formatted_performances,
    "total_commissions": to_string(total_commissions),
    "top_performer_employee_id": top_performer_id,
    "top_performer_sales": to_string(top_performer_sales)
  }

  return report_data

def get_performance_report_logic(
  db: Session,
  date_range: object,
  employee_id: int | None
) -> dict:
  p_employee_id = employee_id if employee_id is not None else "all"
  cache_key = (
    f"reports:employee-report:"
    f"start={date_range.start_date}:end={date_range.end_date}:"
    f"emp={p_employee_id}"
  )

  cached_data = get_cache(cache_key)

  if cached_data:
    print("CACHE_HIT")
    return cached_data

  try:
    db_data = _get_performance_report_from_db(
      db=db,
      date_range=date_range,
      employee_id=employee_id
    )
  except Exception as e:
    print(f"Error querying database: {e}")
    raise e

  if db_data:
    set_cache(
      cache_key,
      db_data,
      CACHE_TTL
    )
  
  return db_data

def generate_employee_performance_logic(
    analytics_db: Session,
    calculation_id: int
) -> dict:
    
    # 1. Ambil Data Komisi (dari tabel staging Pub/Sub)
    commission_data = analytics_db.query(RawFinalizedCommission).filter(
        RawFinalizedCommission.calculation_id == calculation_id
    ).first()
    
    if not commission_data:
        raise ValueError(f"Data komisi final untuk CalculationID {calculation_id} tidak ditemukan.")
    
    # 2. Ambil Data Penjualan (dari tabel mentah order)
    sales_metrics = etl_repo.get_sales_metrics_for_period(
        analytics_db,
        commission_data.employee_id,
        commission_data.period_start,
        commission_data.period_end
    )

    # 3. Gabungkan Semua Data
    final_data = {
        "calculation_id": commission_data.calculation_id,
        "employee_id": commission_data.employee_id,
        "period_start": commission_data.period_start,
        "period_end": commission_data.period_end,
        "total_sales": commission_data.total_sales, # Dari event komisi
        "commission_earned": commission_data.commission_earned, # Dari event komisi
        "total_transactions": sales_metrics['total_transactions'], # Dari ETL
        "total_items_sold": sales_metrics['total_items_sold'] # Dari ETL
    }

    try:
        # 4. LOAD (Upsert ke tabel ringkasan akhir)
        etl_repo.upsert_employee_performance(analytics_db, final_data)
        analytics_db.commit()
    except Exception as e:
        analytics_db.rollback()
        print(f"Error saat upsert data employee performance: {e}")
        raise e
        
    # 5. Kembalikan data yang baru saja diproses
    return final_data
import datetime
from sqlalchemy.orm import Session
from sqlalchemy import text
from decimal import Decimal

# --- Analytics DB ---
def get_kpi_for_date(
  db: Session,
  date: datetime.date
) -> dict:
  params = {
    "date": date
  }

  query_str = """
    SELECT
      SUM(net_sales) as total_revenue,
      SUM(gross_profit) as total_gross_profit,
      SUM(total_transactions) as total_transactions,
      SUM(total_items_sold) as total_items_sold
    FROM sales_summary_daily
    WHERE date = :date 
  """
  
  result = db.execute(text(query_str), params).first()
  return dict(result._mapping) if result and result.total_revenue is not None else {}

def get_top_products_for_date(
  db: Session,
  date: datetime.date,
  limit: int = 5
) -> list[dict]:
  params = {
    "date": date,
    "limit": limit
  }

  query_str = """
    SELECT 
      product_code, 
      SUM(net_sales) as total_net_sales
    FROM product_sales_summary
    WHERE date = :date
    GROUP BY product_code
    ORDER BY total_net_sales DESC
    LIMIT :limit
  """

  result = db.execute(text(query_str), params).all()
  return [dict(row._mapping) for row in result]

# def get_top_performers_for_date(
#   db: Session,
#   date: datetime.date,
#   limit: int = 5
# ) -> list[dict]:
#   params = {
#     # Kita menggunakan 'date' untuk kedua parameter
#     "date_today": date, 
#     "limit": limit
#   }

#   query_str = """
#     SELECT 
#       employee_id, 
#       total_sales,
#       commission_earned
#     FROM employee_performance
#     WHERE 
#       period_start <= :date_today 
#       AND period_end >= :date_today
#     ORDER BY total_sales DESC
#     LIMIT :limit
#   """

#   result = db.execute(text(query_str), params).all()
#   return [dict(row._mapping) for row in result]

# def get_top_performers_for_date(
#     db: Session, 
#     date: datetime.date, 
#     limit: int = 5
# ) -> list[dict]:
#     # Kita harus join antara order items (untuk ambil sales) 
#     # dan documents (untuk ambil tanggal)
#     # Lalu GROUP BY employee_id saja.
    
#     query_str = """
#         SELECT 
#             oi.serving_employee_id as employee_id,
#             SUM(oi.line_total) as total_sales
#         FROM raw_order_items oi
#         JOIN raw_order_documents od ON oi.document_number = od.document_number
#         WHERE CAST(od.order_timestamp AS DATE) = :date
#           AND oi.serving_employee_id IS NOT NULL
#         GROUP BY oi.serving_employee_id -- <--- PENTING: Grouping hanya berdasarkan ID Karyawan
#         ORDER BY total_sales DESC
#         LIMIT :limit
#     """
    
#     params = {
#         "date": date,
#         "limit": limit
#     }
    
#     result = db.execute(text(query_str), params).all()
#     return [dict(row._mapping) for row in result]

def get_top_performers_for_date(db: Session, date: datetime.date, limit: int = 5) -> list[dict]:
    query_str = """
        SELECT 
            oi.serving_employee_id as employee_id,
            -- Ambil nama dari tabel lokal
            COALESCE(e.name, 'Unknown') as employee_name, 
            SUM(oi.line_total) as total_sales
        FROM raw_order_items oi
        JOIN raw_order_documents od ON oi.document_number = od.document_number
        -- JOIN ke tabel raw_employees lokal
        LEFT JOIN raw_employees e ON oi.serving_employee_id = e.employee_id
        WHERE CAST(od.order_timestamp AS DATE) = :date
          AND oi.serving_employee_id IS NOT NULL
        GROUP BY oi.serving_employee_id, e.name
        ORDER BY total_sales DESC
        LIMIT :limit
    """
    
    params = {"date": date, "limit": limit}
    result = db.execute(text(query_str), params).all()
    return [dict(row._mapping) for row in result]

# --- Inventory DB ---
def get_low_stock_alerts(db: Session) -> list[str]:
  query_str = """
    SELECT 
      p.product_name,
      s.available_quantity,
      p.reorder_level
    FROM stocks s
    JOIN inventory_products p ON s.product_code = p.product_code
    WHERE s.available_quantity < p.reorder_level
    ORDER BY s.available_quantity ASC;
  """

  try:
    result = db.execute(text(query_str)).all()
    alerts = [
      f"{row.product_name} (Left: {row.available_quantity}, Limit: {row.reorder_level})" for row in result
    ]
    return alerts
  except Exception as e:
    print(f"Error fetching stock data: {e}")
    return []

# --- Commissions DB ---
def get_pending_commissions_count(db: Session) -> int:
  query_str = """
    SELECT COUNT(*) 
    FROM commission_calculations
    WHERE status IN (1, 2, 3);
  """

  try:
    result = db.execute(text(query_str)).scalar_one_or_none()
    return result or 0
  except Exception as e:
    print(f"Error fetching commissions data: {e}")
    return 0

# --- POS DB ---
def get_realtime_metrics_from_pos(db: Session) -> dict:
  query_str = """
    SELECT
      SUM(CAST(total_amount AS decimal)) as hourly_revenue,
      COUNT(id) as hourly_transaction_count
    FROM order_documents
    WHERE orders_date >= NOW() - INTERVAL '60 minutes';
  """

  result = db.execute(text(query_str)).first()
  return dict(result._mapping) if result else {}

def get_active_transactions_count(db: Session) -> int:
  query_str = """
    SELECT COUNT(id) 
    FROM order_documents
    WHERE paid_status = 0;
  """

  result = db.execute(text(query_str)).scalar_one_or_none()
  return result or 0

def get_recent_large_transactions(
  db: Session,
  threshold: Decimal,
  limit: int = 5
) -> list[dict]:
  params = {
    "threshold": threshold,
    "limit": limit
  }
  query_str = """
    SELECT id, total_amount, cashier_id
    FROM order_documents
    WHERE CAST(total_amount AS decimal) > :threshold
    ORDER BY orders_date DESC
    LIMIT :limit;
  """

  result = db.execute(text(query_str), params).all()
  return [dict(row._mapping) for row in result]
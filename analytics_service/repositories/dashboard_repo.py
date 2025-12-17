import datetime
from sqlalchemy.orm import Session
from sqlalchemy import text
from decimal import Decimal

# --- Analytics DB ---
# def get_kpi_for_date(
#   db: Session,
#   date: datetime.date
# ) -> dict:
#   params = {
#     "date": date
#   }

#   query_str = """
#     SELECT
#       SUM(net_sales) as total_revenue,
#       SUM(gross_profit) as total_gross_profit,
#       SUM(total_transactions) as total_transactions,
#       SUM(total_items_sold) as total_items_sold
#     FROM sales_summary_daily
#     WHERE date = :date 
#   """
  
#   result = db.execute(text(query_str), params).first()
#   return dict(result._mapping) if result and result.total_revenue is not None else {}

def get_kpi_for_date(db: Session, date_obj: datetime.date) -> dict:
    today = datetime.date.today()
    
    # === STRATEGI 1: REAL-TIME (Untuk Hari Ini) ===
    if date_obj == today:
        # QUERY 1: Ambil Revenue & Transaksi dari tabel 'raw_sales_events'
        # Sesuai models.py: RawSalesEvent punya kolom 'total_amount' dan 'order_timestamp'
        query_header = text("""
            SELECT 
                COALESCE(SUM(total_amount), 0) as total_revenue,
                COUNT(id) as total_transactions
            FROM raw_sales_events
            -- Menggunakan Timezone Asia/Jakarta agar akurat dengan jam lokal
            WHERE DATE(order_timestamp AT TIME ZONE 'Asia/Jakarta') = :date
        """)
        header_result = db.execute(query_header, {"date": date_obj}).first()
        
        # QUERY 2: Ambil Items Sold & Profit dari tabel 'raw_order_items'
        # Kita perlu JOIN ke 'raw_sales_events' hanya untuk memfilter berdasarkan tanggal transaksi
        # Sesuai models.py: RawOrderItem punya 'quantity', 'line_total', 'cost_price'
        query_items = text("""
            SELECT 
                COALESCE(SUM(i.quantity), 0) as total_items_sold,
                -- Profit = Total Jual - (Qty * Harga Modal)
                COALESCE(SUM(i.line_total - (i.quantity * i.cost_price)), 0) as total_gross_profit
            FROM raw_order_items i
            JOIN raw_sales_events se ON i.document_number = se.order_document_number
            WHERE DATE(se.order_timestamp AT TIME ZONE 'Asia/Jakarta') = :date
        """)
        items_result = db.execute(query_items, {"date": date_obj}).first()

        # GABUNGKAN HASILNYA
        return {
            "total_revenue": header_result.total_revenue if header_result else 0,
            "total_transactions": header_result.total_transactions if header_result else 0,
            "total_items_sold": items_result.total_items_sold if items_result else 0,
            "total_gross_profit": items_result.total_gross_profit if items_result else 0
        }

    # === STRATEGI 2: HISTORICAL (Untuk Kemarin/Lampau) ===
    else:
        # Menggunakan SalesSummaryDaily sesuai models.py
        query = text("""
            SELECT 
                gross_sales as total_revenue, -- atau net_sales tergantung definisi bisnis Anda
                total_transactions, 
                total_items_sold, 
                gross_profit as total_gross_profit
            FROM sales_summary_daily 
            WHERE date = :date
        """)
        result = db.execute(query, {"date": date_obj}).first()
        return dict(result._mapping) if result else {}
    
# def get_top_products_for_date(
#   db: Session,
#   date: datetime.date,
#   limit: int = 5
# ) -> list[dict]:
#   params = {
#     "date": date,
#     "limit": limit
#   }

#   query_str = """
#     SELECT 
#       product_code, 
#       SUM(net_sales) as total_net_sales
#     FROM product_sales_summary
#     WHERE date = :date
#     GROUP BY product_code
#     ORDER BY total_net_sales DESC
#     LIMIT :limit
#   """

#   result = db.execute(text(query_str), params).all()
#   return [dict(row._mapping) for row in result]

def get_top_products_for_date(
    db: Session, 
    date: datetime.date, 
    limit: int = 1
) -> list[dict]:
    
    query_str = """
      SELECT 
        oi.product_code,
        COALESCE(MAX(p.product_name), 'Unknown Product') as product_name,
        SUM(oi.quantity) as quantity_sold,
        SUM(oi.line_total) as net_sales,
        SUM(oi.line_total - (oi.quantity * oi.cost_price)) as gross_profit

      FROM raw_order_items oi
      JOIN raw_order_documents od ON oi.document_number = od.document_number
      LEFT JOIN raw_products p ON oi.product_code = p.product_code
      
      WHERE CAST(od.order_timestamp AS DATE) = :date
      
      GROUP BY oi.product_code
      ORDER BY net_sales DESC
      LIMIT :limit
    """
    
    params = {
      "date": date,
      "limit": limit
    }
    
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

def get_top_performers_for_date(
    db: Session, 
    date: datetime.date, 
    limit: int = 5
) -> list[dict]:
    # Kita harus join antara order items (untuk ambil sales) 
    # dan documents (untuk ambil tanggal)
    # Lalu GROUP BY employee_id saja.
    
    query_str = """
        SELECT 
            oi.serving_employee_id as employee_id,
            SUM(oi.line_total) as total_sales
        FROM raw_order_items oi
        JOIN raw_order_documents od ON oi.document_number = od.document_number
        WHERE CAST(od.order_timestamp AS DATE) = :date
          AND oi.serving_employee_id IS NOT NULL
        GROUP BY oi.serving_employee_id -- <--- PENTING: Grouping hanya berdasarkan ID Karyawan
        ORDER BY total_sales DESC
        LIMIT :limit
    """
    
    params = {
        "date": date,
        "limit": limit
    }
    
    result = db.execute(text(query_str), params).all()
    return [dict(row._mapping) for row in result]

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

def get_top_products_by_range(
    db: Session, 
    start_date: datetime.date, 
    end_date: datetime.date, 
    limit: int = 5
) -> list[dict]:
    
    query_str = """
        SELECT 
            oi.product_code,
            COALESCE(MAX(oi.product_name), MAX(p.product_name), 'Unknown Product') as product_name,
            SUM(oi.quantity) as quantity_sold,
            SUM(oi.line_total) as net_sales
        FROM raw_order_items oi
        JOIN raw_order_documents od ON oi.document_number = od.document_number
        LEFT JOIN raw_products p ON oi.product_code = p.product_code
        
        -- UBAH DI SINI: Filter berdasarkan Range Tanggal (Bulanan)
        WHERE CAST(od.order_timestamp AS DATE) BETWEEN :start_date AND :end_date
        
        GROUP BY oi.product_code
        ORDER BY net_sales DESC
        LIMIT :limit
    """
    
    params = {
        "start_date": start_date,
        "end_date": end_date,
        "limit": limit
    }
    
    result = db.execute(text(query_str), params).all()
    return [dict(row._mapping) for row in result]

# Lakukan hal yang sama untuk get_top_performers_by_range
def get_top_performers_by_range(
    db: Session, 
    start_date: datetime.date, 
    end_date: datetime.date, 
    limit: int = 5
) -> list[dict]:
    
    query_str = """
        SELECT 
            oi.serving_employee_id as employee_id,
            COALESCE(e.name, 'Unknown') as employee_name, 
            SUM(oi.line_total) as total_sales
        FROM raw_order_items oi
        JOIN raw_order_documents od ON oi.document_number = od.document_number
        LEFT JOIN raw_employees e ON oi.serving_employee_id = e.employee_id
        
        -- UBAH DI SINI: Filter Range
        WHERE CAST(od.order_timestamp AS DATE) BETWEEN :start_date AND :end_date
          AND oi.serving_employee_id IS NOT NULL
          
        GROUP BY oi.serving_employee_id, e.name
        ORDER BY total_sales DESC
        LIMIT :limit
    """
    
    params = {"start_date": start_date, "end_date": end_date, "limit": limit}
    result = db.execute(text(query_str), params).all()
    return [dict(row._mapping) for row in result]
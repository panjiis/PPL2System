# repositories/etl_repo.py
from sqlalchemy.orm import Session
from sqlalchemy import text
import datetime
# from .models import RawFinalizedCommission, EmployeePerformance

def get_raw_sales_data(
    db: Session,
    date: datetime.date,
    cashier_id: int | None
) -> list[dict]:
    params = {"date": date}
    query_str = """
        SELECT
            COALESCE(da.date, ia.date) as date,
            COALESCE(da.cashier_id, ia.cashier_id) as cashier_id,
            COALESCE(da.total_transactions, 0) as total_transactions,
            COALESCE(ia.total_items_sold, 0) as total_items_sold,
            COALESCE(ia.gross_sales, 0) as gross_sales,
            COALESCE(ia.total_discounts, 0) as total_discounts,
            COALESCE(ia.net_sales, 0) as net_sales,
            COALESCE(da.total_tax, 0) as total_tax,
            COALESCE(ia.total_cost, 0) as total_cost,
            (COALESCE(ia.net_sales, 0) - COALESCE(ia.total_cost, 0)) as gross_profit
        FROM (
            -- Subquery Dokumen (dari raw_order_documents)
            SELECT
                CAST(od.order_timestamp AS date) as date,
                od.cashier_id,
                COUNT(od.id) as total_transactions,
                SUM(od.tax_amount) as total_tax
            FROM raw_order_documents od -- <-- TABEL LOKAL
            WHERE CAST(od.order_timestamp AS date) = :date
    """
    
    if cashier_id:
        query_str += " AND od.cashier_id = :cashier_id"
        params["cashier_id"] = cashier_id
        
    query_str += """
            GROUP BY CAST(od.order_timestamp AS date), od.cashier_id
        ) da
        FULL OUTER JOIN (
            -- Subquery Item (dari raw_order_items)
            SELECT
                CAST(od.order_timestamp AS date) as date,
                od.cashier_id,
                SUM(oi.quantity) as total_items_sold,
                SUM(oi.price_before_discount) as gross_sales,
                SUM(oi.discount_amount) as total_discounts,
                SUM(oi.line_total) as net_sales,
                SUM(oi.quantity * oi.cost_price) as total_cost -- <-- DARI ITEM LOKAL
            FROM raw_order_items oi -- <-- TABEL LOKAL
            JOIN raw_order_documents od ON oi.document_number = od.document_number -- <-- JOIN LOKAL
            WHERE CAST(od.order_timestamp AS date) = :date
    """
    
    if cashier_id:
        query_str += " AND od.cashier_id = :cashier_id"
        params["cashier_id"] = cashier_id
        
    query_str += """
            GROUP BY CAST(od.order_timestamp AS date), od.cashier_id
        ) ia ON da.date = ia.date AND da.cashier_id = ia.cashier_id
    """

    result = db.execute(text(query_str), params).all()
    return [dict(row._mapping) for row in result]

def upsert_sales_summary_daily(
    analytics_db: Session, 
    summary_data: dict
) -> int:
    upsert_query = text("""
        INSERT INTO public.sales_summary_daily (
            date, cashier_id, total_transactions, total_items_sold,
            gross_sales, total_discounts, net_sales, total_tax,
            total_cost, gross_profit, created_at, updated_at
        )
        VALUES (
            :date, :cashier_id, :total_transactions, :total_items_sold,
            :gross_sales, :total_discounts, :net_sales, :total_tax,
            :total_cost, :gross_profit, NOW(), NOW()
        )
        ON CONFLICT (date, cashier_id) DO UPDATE SET
            total_transactions = EXCLUDED.total_transactions,
            total_items_sold = EXCLUDED.total_items_sold,
            gross_sales = EXCLUDED.gross_sales,
            total_discounts = EXCLUDED.total_discounts,
            net_sales = EXCLUDED.net_sales,
            total_tax = EXCLUDED.total_tax,
            total_cost = EXCLUDED.total_cost,
            gross_profit = EXCLUDED.gross_profit,
            updated_at = NOW()
        RETURNING id;
    """)
    
    result = analytics_db.execute(upsert_query, summary_data)
    analytics_db.commit() 
    
    return result.scalar_one()

def get_raw_product_sales_data(
    analytics_db: Session,
    date: datetime.date,
    product_code: str | None,
    product_group_id: int | None
) -> list[dict]:
    
    params = {"date": date}
    query_str = """
        SELECT
            CAST(od.order_timestamp AS date) as date,
            oi.product_code,
            p.product_code,
            p.product_group_id,
            SUM(oi.quantity) as quantity_sold,
            SUM(oi.price_before_discount) as gross_sales,
            SUM(oi.discount_amount) as total_discounts,
            SUM(oi.line_total) as net_sales,
            SUM(oi.quantity * oi.cost_price) as total_cost,
            (SUM(oi.line_total) - SUM(oi.quantity * oi.cost_price)) as gross_profit
        FROM raw_order_items oi
        JOIN raw_order_documents od ON oi.document_number = od.document_number
        -- JOIN krusial ke tabel produk baru kita
        JOIN raw_products p ON oi.product_code = p.product_code 
        WHERE CAST(od.order_timestamp AS date) = :date
    """
    
    if product_code:
        query_str += " AND p.product_code = :product_code"
        params["product_code"] = product_code
    
    if product_group_id:
        query_str += " AND p.product_group_id = :product_group_id"
        params["product_group_id"] = product_group_id
        
    query_str += """
        GROUP BY CAST(od.order_timestamp AS date), p.product_code, p.product_group_id, oi.product_code
    """

    result = analytics_db.execute(text(query_str), params).all()
    return [dict(row._mapping) for row in result]

# --- FUNGSI 2: MELAKUKAN UPSERT ---
def upsert_product_sales_summary(
    analytics_db: Session, 
    summary_data: dict
) -> int:
    upsert_query = text("""
        INSERT INTO public.product_sales_summary (
            date, product_code, product_group_id, quantity_sold,
            gross_sales, total_discounts, net_sales, total_cost,
            gross_profit, created_at, updated_at
        )
        VALUES (
            :date, :product_code, :product_group_id, :quantity_sold,
            :gross_sales, :total_discounts, :net_sales, :total_cost,
            :gross_profit, NOW(), NOW()
        )
        ON CONFLICT (date, product_code) DO UPDATE SET
            quantity_sold = product_sales_summary.quantity_sold + EXCLUDED.quantity_sold,
            gross_sales = product_sales_summary.gross_sales + EXCLUDED.gross_sales,
            total_discounts = product_sales_summary.total_discounts + EXCLUDED.total_discounts,
            net_sales = product_sales_summary.net_sales + EXCLUDED.net_sales,
            total_cost = product_sales_summary.total_cost + EXCLUDED.total_cost,
            gross_profit = product_sales_summary.gross_profit + EXCLUDED.gross_profit,
            updated_at = NOW()
        RETURNING id;
    """)
    
    result = analytics_db.execute(upsert_query, summary_data)
    
    return result.scalar_one()

def get_raw_customer_analytics_data(
    analytics_db: Session,
    date: datetime.date,
    product_group_id: int | None
) -> list[dict]:
    
    params = {"date": date}
    
    query_str = """
        SELECT
            CAST(od.order_timestamp AS date) as date,
            p.product_group_id,
            COUNT(DISTINCT od.document_number) as total_transactions,
            SUM(oi.line_total) as total_revenue
        FROM raw_order_items oi
        JOIN raw_order_documents od ON oi.document_number = od.document_number
        JOIN raw_products p ON oi.product_code = p.product_code 
        WHERE CAST(od.order_timestamp AS date) = :date
    """
    
    if product_group_id:
        query_str += " AND p.product_group_id = :product_group_id"
        params["product_group_id"] = product_group_id
        
    query_str += """
        GROUP BY CAST(od.order_timestamp AS date), p.product_group_id
    """

    result = analytics_db.execute(text(query_str), params).all()
    return [dict(row._mapping) for row in result]

def upsert_customer_analytics(
    analytics_db: Session, 
    summary_data: dict
) -> int:
    upsert_query = text("""
        INSERT INTO public.customer_analytics (
            date, product_group_id, total_transactions, total_revenue,
            average_transaction_value, created_at, updated_at
        )
        VALUES (
            :date, :product_group_id, :total_transactions, :total_revenue,
            :average_transaction_value, NOW(), NOW()
        )
        ON CONFLICT (date, product_group_id) DO UPDATE SET
            total_transactions = EXCLUDED.total_transactions,
            total_revenue = EXCLUDED.total_revenue,
            average_transaction_value = EXCLUDED.average_transaction_value,
            updated_at = NOW()
        RETURNING id;
    """)
    
    result = analytics_db.execute(upsert_query, summary_data)
    return result.scalar_one()

def get_sales_metrics_for_period(
    analytics_db: Session,
    employee_id: int,
    start_date: datetime.date,
    end_date: datetime.date
) -> dict:
    """
    Mengagregasi raw_order_items untuk mendapatkan total transaksi dan item
    untuk seorang karyawan dalam satu periode.
    """
    params = {
        "employee_id": employee_id,
        "start_date": start_date,
        "end_date": end_date
    }
    
    # Kueri ini hanya menghitung dari data mentah order
    query_str = """
        SELECT
            COUNT(DISTINCT oi.document_number) as total_transactions,
            SUM(oi.quantity) as total_items_sold
        FROM raw_order_items oi
        JOIN raw_order_documents od ON oi.document_number = od.document_number
        WHERE 
            -- Hanya untuk karyawan yang melayani (serving employee)
            oi.serving_employee_id = :employee_id
            AND CAST(od.order_timestamp AS date) BETWEEN :start_date AND :end_date
    """
    
    result = analytics_db.execute(text(query_str), params).first()
    
    if result and result.total_transactions > 0:
        return dict(result._mapping)
    else:
        return {"total_transactions": 0, "total_items_sold": 0}

# --- FUNGSI 2: MELAKUKAN UPSERT FINAL ---
def upsert_employee_performance(
    analytics_db: Session, 
    performance_data: dict
) -> int:
    # Menggunakan ON CONFLICT pada 'calculation_id' yang unik
    upsert_query = text("""
        INSERT INTO public.employee_performance (
            calculation_id, employee_id, period_start, period_end,
            total_sales, total_transactions, total_items_sold, 
            commission_earned, created_at, updated_at
        )
        VALUES (
            :calculation_id, :employee_id, :period_start, :period_end,
            :total_sales, :total_transactions, :total_items_sold, 
            :commission_earned, NOW(), NOW()
        )
        ON CONFLICT (calculation_id) DO UPDATE SET
            total_sales = EXCLUDED.total_sales,
            total_transactions = EXCLUDED.total_transactions,
            total_items_sold = EXCLUDED.total_items_sold,
            commission_earned = EXCLUDED.commission_earned,
            updated_at = NOW()
        RETURNING id;
    """)
    
    result = analytics_db.execute(upsert_query, performance_data)
    return result.scalar_one()

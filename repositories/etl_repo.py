# repositories/etl_repo.py
from sqlalchemy.orm import Session
from sqlalchemy import text
import datetime

def get_raw_sales_data_from_pos(
    pos_db: Session,
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
            SELECT
                CAST(od.orders_date AS date) as date,
                od.cashier_id,
                COUNT(od.id) as total_transactions,
                SUM(CAST(od.tax_amount AS decimal)) as total_tax
            FROM order_documents od
            WHERE CAST(od.orders_date AS date) = :date
    """
    
    if cashier_id:
        query_str += " AND od.cashier_id = :cashier_id"
        params["cashier_id"] = cashier_id
        
    query_str += """
            GROUP BY CAST(od.orders_date AS date), od.cashier_id
        ) da
        FULL OUTER JOIN (
            SELECT
                CAST(od.orders_date AS date) as date,
                od.cashier_id,
                SUM(oi.quantity) as total_items_sold,
                SUM(CAST(oi.price_before_discount AS decimal)) as gross_sales,
                SUM(CAST(oi.discount_amount AS decimal)) as total_discounts,
                SUM(CAST(oi.line_total AS decimal)) as net_sales,
                SUM(oi.quantity * CAST(p.cost_price AS decimal)) as total_cost
            FROM order_items oi
            JOIN order_documents od ON oi.document_id = od.id
            JOIN products p ON oi.product_code = p.product_code
            WHERE CAST(od.orders_date AS date) = :date
    """
    
    if cashier_id:
        query_str += " AND od.cashier_id = :cashier_id"
        params["cashier_id"] = cashier_id
        
    query_str += """
            GROUP BY CAST(od.orders_date AS date), od.cashier_id
        ) ia ON da.date = ia.date AND da.cashier_id = ia.cashier_id
    """

    result = pos_db.execute(text(query_str), params).all()
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


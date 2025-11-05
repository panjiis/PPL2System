# repositories/sales_repo.py
from sqlalchemy.orm import Session
from sqlalchemy import text
import datetime
from decimal import Decimal

def get_main_aggregates(
    db: Session,
    start_date: datetime.date,
    end_date: datetime.date,
    cashier_id: int | None,
    product_group_id: int | None
) -> dict:
    params = {"start_date": start_date, "end_date": end_date}
    
    if product_group_id:
        params["product_group_id"] = product_group_id
        query_str = """
            SELECT
                SUM(net_sales) as total_revenue,
                SUM(total_cost) as total_cost,
                SUM(gross_profit) as gross_profit,
                0 as total_transactions, -- Tabel ini tidak punya data transaksi unik
                SUM(quantity_sold) as total_items_sold
            FROM product_sales_summary
            WHERE date BETWEEN :start_date AND :end_date
              AND product_group_id = :product_group_id
        """
    else:
        query_str = """
            SELECT
                SUM(net_sales) as total_revenue,
                SUM(total_cost) as total_cost,
                SUM(gross_profit) as gross_profit,
                SUM(total_transactions) as total_transactions,
                SUM(total_items_sold) as total_items_sold
            FROM sales_summary_daily
            WHERE date BETWEEN :start_date AND :end_date
        """
        if cashier_id:
            query_str += " AND cashier_id = :cashier_id"
            params["cashier_id"] = cashier_id

    result = db.execute(text(query_str), params).first()
    return dict(result._mapping) if result and result.total_revenue is not None else {}

def get_daily_breakdown(
    db: Session,
    start_date: datetime.date,
    end_date: datetime.date,
    cashier_id: int | None
) -> list[dict]:
    params = {"start_date": start_date, "end_date": end_date}
    query_str = """
        SELECT * FROM sales_summary_daily
        WHERE date BETWEEN :start_date AND :end_date
    """
    
    if cashier_id:
        query_str += " AND cashier_id = :cashier_id"
        params["cashier_id"] = cashier_id
        
    query_str += " ORDER BY date"
    
    result = db.execute(text(query_str), params).all()
    return [dict(row._mapping) for row in result]


import datetime
from sqlalchemy.orm import Session
from sqlalchemy import text

def get_product_breakdown(
    db: Session,
    start_date: datetime.date,
    end_date: datetime.date,
    product_group_id: int | None,
    limit: int = 5  
) -> list[dict]:
    params = {"start_date": start_date, "end_date": end_date, "limit": limit}
    
    query_str = """
        SELECT
            pss.product_id,
            pss.product_group_id,
            SUM(pss.quantity_sold) as total_quantity_sold,
            SUM(pss.gross_sales) as total_gross_sales,
            SUM(pss.total_discounts) as total_discounts_given,
            SUM(pss.net_sales) as total_net_sales,
            SUM(pss.total_cost) as total_cost_of_goods,
            SUM(pss.gross_profit) as total_gross_profit
        FROM product_sales_summary pss
        WHERE pss.date BETWEEN :start_date AND :end_date
    """
    
    if product_group_id:
        query_str += " AND pss.product_group_id = :product_group_id"
        params["product_group_id"] = product_group_id
        
    query_str += """
        GROUP BY pss.product_id, pss.product_group_id --, p.product_name
        ORDER BY total_net_sales DESC
        LIMIT :limit
    """
    
    result = db.execute(text(query_str), params).all()
    
    return [dict(row._mapping) for row in result]

def get_daily_summary_by_date(
    db: Session, 
    date: datetime.date,
    cashier_id: int | None
) -> list[dict]:
    params = {"date": date}
    
    query_str = """
        SELECT * FROM sales_summary_daily
        WHERE date = :date
    """
    
    if cashier_id:
        query_str += " AND cashier_id = :cashier_id"
        params["cashier_id"] = cashier_id
        
    query_str += " ORDER BY id"
    
    result = db.execute(text(query_str), params).all()
    
    return [dict(row._mapping) for row in result]

def get_product_sales_paginated(
    db: Session,
    start_date: datetime.date,
    end_date: datetime.date,
    product_id: int | None,
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
        SELECT * FROM product_sales_summary
        WHERE date BETWEEN :start_date AND :end_date
          AND id > :last_id
    """

    if product_id:
        query_str += " AND product_id = :product_id"
        params["product_id"] = product_id
    
    if product_group_id:
        query_str += " AND product_group_id = :product_group_id"
        params["product_group_id"] = product_group_id
    
    query_str += " ORDER BY id LIMIT :page_size"

    result = db.execute(text(query_str), params).all()
    return [dict(row._mapping) for row in result]

def count_total_product_sales(
    db: Session,
    start_date: datetime.date,
    end_date: datetime.date,
    product_id: int | None,
    product_group_id: int | None
) -> int:
    params = {
        "start_date": start_date,
        "end_date": end_date
    }

    query_str = """
        SELECT COUNT(*) FROM product_sales_summary
        WHERE date BETWEEN :start_date AND :end_date
    """

    if product_id:
        query_str += " AND product_id = :product_id"
        params["product_id"] = product_id
    
    if product_group_id:
        query_str += " AND product_group_id = :product_group_id"
        params["product_group_id"] = product_group_id
    
    result = db.execute(text(query_str), params).scalar_one_or_none()
    return result or 0
# core_logic/sales.py
from sqlalchemy.orm import Session
from repositories import sales_repo, etl_repo
import datetime
from decimal import Decimal, ROUND_HALF_UP
from core_logic.utils import *
from core_logic.cache_manager import *

CACHE_TTL = 1800

def _fetch_sales_report_from_db(
    db: Session,
    date_range: object,
    cashier_id: int | None,
    product_group_id: int | None,
    include_daily_breakdown: bool,
    include_product_breakdown: bool
) -> dict:
    try:
        start_date = datetime.date.fromisoformat(date_range.start_date)
        end_date = datetime.date.fromisoformat(date_range.end_date)
    except ValueError:
        raise ValueError("Incorrect date format in date_range. Use YYYY-MM-DD.")
    
    main_aggr = sales_repo.get_main_aggregates(
        db, start_date, end_date, cashier_id, product_group_id 
    )

    total_revenue = main_aggr.get('total_revenue', Decimal(0))
    total_cost = main_aggr.get('total_cost', Decimal(0))
    gross_profit = main_aggr.get('gross_profit', Decimal(0))
    total_transactions = main_aggr.get('total_transactions', 0)
    total_items_sold = main_aggr.get('total_items_sold', 0)

    profit_margin = (
        (gross_profit / total_revenue) * Decimal(100) if total_revenue > 0 else Decimal(0)
    )

    avg_tx_value = (
        (total_revenue / total_transactions) * Decimal(100) if total_revenue > 0 else Decimal(0)
    )

    report_output = {
        "period": {
            "start_date": date_range.start_date,
            "end_date": date_range.end_date
        },
        "summary": {
            "total_revenue": to_string(total_revenue),
            "total_cost": to_string(total_cost),
            "gross_profit": to_string(gross_profit),
            "profit_margin_percentage": to_percent_string(profit_margin),
            "total_transactions": total_transactions,
            "total_items_sold": total_items_sold,
            "average_transaction_value": to_string(avg_tx_value)
        },
        "daily_breakdowns": [],
        "top_products": []
    }

    if include_daily_breakdown and not product_group_id:
        daily_data = sales_repo.get_daily_breakdown(
            db, start_date, end_date, cashier_id
        )
        report_output["daily_breakdowns"] = daily_data
    
    if include_product_breakdown:
        product_data = sales_repo.get_product_breakdown(
            db, start_date, end_date, product_group_id, limit=5
        )
        report_output["top_products"] = product_data

    return report_output

def generate_sales_report_logic(
    db: Session,
    date_range: object, 
    cashier_id: int | None,
    product_group_id: int | None,
    include_daily_breakdown: bool,
    include_product_breakdown: bool
) -> dict:
    p_cashier_id = cashier_id if cashier_id is not None else "all"
    p_group_id = product_group_id if product_group_id is not None else "all"

    cache_key = (
        f"reports:sales:"
        f"start={date_range.start_date}:end={date_range.end_date}:"
        f"cashier={p_cashier_id}:group={p_group_id}:"
        f"daily={include_daily_breakdown}:product={include_product_breakdown}"
    )

    cached_data = get_cache(cache_key)

    if cached_data:
        print("CACHE HIT")
        return cached_data

    try:
        db_data = _fetch_sales_report_from_db(
            db=db,
            date_range=date_range,
            cashier_id=cashier_id,
            product_group_id=product_group_id,
            include_daily_breakdown=include_daily_breakdown,
            include_product_breakdown=include_product_breakdown
        )
    
    except Exception as e:
        print(f"Error querying database: {e}")
        raise e
    
    if db_data:
        set_cache(
            key=cache_key,
            value=db_data,
            ttl_seconds=CACHE_TTL
        )

    return db_data

def _get_daily_summary_logic_from_db(
    db: Session, 
    date_str: str, 
    cashier_id: int | None
) -> list[dict]:
    try:
        parsed_date = datetime.date.fromisoformat(date_str)
    except ValueError:
        raise ValueError(f"Incorrect date format: {date_str}. Use YYYY-MM-DD.")
        
    raw_summaries = sales_repo.get_daily_summary_by_date(
        db, parsed_date, cashier_id
    )
    
    formatted_summaries = []
    for item in raw_summaries:
        formatted_summaries.append({
            "id": item['id'],
            "date": item['date'].isoformat(),
            "cashier_id": item['cashier_id'],
            "total_transactions": item['total_transactions'],
            "total_items_sold": item['total_items_sold'],
            "gross_sales": to_string(item['gross_sales']),
            "total_discounts": to_string(item['total_discounts']),
            "net_sales": to_string(item['net_sales']),
            "total_tax": to_string(item['total_tax']),
            "total_cost": to_string(item['total_cost']),
            "gross_profit": to_string(item['gross_profit']),
            "created_at": item['created_at'],
            "updated_at": item['updated_at']
        })
        
    return formatted_summaries

def get_daily_summary_logic(
    db: Session,
    date_str: str,
    cashier_id: int | None
) -> list[dict]:
    p_cashier_id = cashier_id if cashier_id is not None else "all"
    cache_key = f"reports:daily-summary:date={date_str}:cashier={p_cashier_id}"

    cached_data = get_cache(cache_key)

    if cached_data:
        print("CACHE HIT")
        return cached_data
    
    try:
        db_data = _get_daily_summary_logic_from_db(
            db=db,
            date_str=date_str,
            cashier_id=cashier_id
        )
    
    except Exception as e:
        print(f"Error querying database:{e}")
        raise e

    if db_data is not None:
        set_cache(
            key=cache_key,
            value=db_data,
            ttl_seconds=CACHE_TTL
        )
    
    return db_data

def generate_daily_summary_logic(
    db: Session, # Hanya butuh DB Analytics
    date_str: str,
    cashier_id: int | None
) -> list[dict]:
    try:
        date = datetime.date.fromisoformat(date_str)
    except ValueError:
        raise ValueError("Incorrect date format. Use YYYY-MM-DD.")
        
    raw_sales_data = etl_repo.get_raw_sales_data(
        db, date, cashier_id # Panggil fungsi baru
    )
    
    if not raw_sales_data:
        print("[Logic] No sales data found for the given date and cashier.")
        return []
        
    generated_ids = []
    for summary_row in raw_sales_data:
        new_id = etl_repo.upsert_sales_summary_daily(db, summary_row)
        generated_ids.append(new_id)
    
    cache_pattern_to_delete = ""
    if cashier_id:
        cache_pattern_to_delete = f"reports:daily-summary:date={date_str}:cashier={cashier_id}"
    else:
        cache_pattern_to_delete = f"reports:daily-summary:date={date_str}:cashier=*"
    
    delete_cache(cache_pattern_to_delete)
        
    final_summaries = get_daily_summary_logic(
        db, date_str, cashier_id
    )
    
    return final_summaries

# def generate_daily_summary_logic(
#     analytics_db: Session,
#     pos_db: Session,
#     date_str: str,
#     cashier_id: int | None
# ) -> list[dict]:
#     try:
#         date = datetime.date.fromisoformat(date_str)
#     except ValueError:
#         raise ValueError("Incorrect date format. Use YYYY-MM-DD.")
        
#     raw_sales_data = etl_repo.get_raw_sales_data_from_pos(
#         pos_db, date, cashier_id
#     )
    
#     if not raw_sales_data:
#         print("[Logic] No sales data found for the given date and cashier.")
#         return []
        
#     generated_ids = []
#     for summary_row in raw_sales_data:
#         new_id = etl_repo.upsert_sales_summary_daily(analytics_db, summary_row)
#         generated_ids.append(new_id)
    
#     cache_pattern_to_delete = ""
#     if cashier_id:
#         cache_pattern_to_delete = f"reports:daily-summary:date={date_str}:cashier={cashier_id}"
#     else:
#         cache_pattern_to_delete = f"reports:daily-summary:date={date_str}:cashier=*"
    
#     delete_cache(cache_pattern_to_delete)
        
#     final_summaries = get_daily_summary_logic(
#         analytics_db, date_str, cashier_id
#     )
    
#     return final_summaries

def get_product_sales_logic(
    db: Session,
    date_range: object,
    product_id: int | None,
    product_group_id: int | None,
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
    
    p_product_id = product_id if product_id is not None else "all"
    p_group_id = product_group_id if product_group_id is not None else "all"

    count_cache_key = (
        f"reports:product-sales:count:"
        f"start={date_range.start_date}:end={date_range.end_date}:"
        f"prod={p_product_id}:group={p_group_id}"
    )

    total_count = get_cache(count_cache_key)

    if total_count is None:
        total_count = sales_repo.count_total_product_sales(
            db, start_date, end_date, product_id, product_group_id
        )
        set_cache(
            count_cache_key,
            total_count,
            CACHE_TTL
        )
    else:
        print("CACHE HIT")
        total_count = int(total_count)

    raw_sales = sales_repo.get_product_sales_paginated(
        db, start_date, end_date, product_id, product_group_id, page_size, last_id
    )

    formatted_sales = []
    for item in raw_sales:
        formatted_sales.append({
            "id": item['id'],
            "date": item['date'].isoformat(),
            "product_id": item['product_id'],
            "product_group_id": item['product_group_id'],
            "quantity_sold": item['quantity_sold'],
            "gross_sales": to_string(item['gross_sales']),
            "total_discounts": to_string(item['total_discounts']),
            "net_sales": to_string(item['net_sales']),
            "total_cost": to_string(item['total_cost']),
            "gross_profit": to_string(item['gross_profit']),
            "created_at": item['created_at'],
            "updated_at": item['updated_at']
        })
    
    next_page_token = ""
    if formatted_sales:
        last_item_id = formatted_sales[-1]['id']
        next_page_token = str(last_item_id)
    
    return {
        "data": formatted_sales,
        "total_count": total_count,
        "next_page_token": next_page_token
    }

def _get_top_selling_products_from_db(
    db: Session,
    date_range: object,
    limit: int,
    product_group_id: int | None
) -> list[dict]:
    try:
        start_date = datetime.date.fromisoformat(date_range.start_date)
        end_date = datetime.date.fromisoformat(date_range.end_date)
    except ValueError:
        raise ValueError("Incorrect date format in date_range. Use YYYY-MM-DD.")
    
    limit_val = limit if limit > 0 else 5

    aggregated_products = sales_repo.get_product_breakdown(
        db, start_date, end_date, product_group_id, limit_val
    )

    formatted_products = []
    for item in aggregated_products:
        formatted_products.append({
            "product_id": item['product_id'],
            "product_group_id": item['product_group_id'],
            "quantity_sold": int(item['total_quantity_sold']), 
            "gross_sales": to_string(item['total_gross_sales']),
            "total_discounts": to_string(item['total_discounts_given']),
            "net_sales": to_string(item['total_net_sales']),
            "total_cost": to_string(item['total_cost_of_goods']),
            "gross_profit": to_string(item['total_gross_profit']),
        })
    
    return formatted_products

def get_top_selling_products_logic(
    db: Session,
    date_range: object,
    limit: int,
    product_group_id: int | None
) -> list[dict]:
    p_group_id = product_group_id if product_group_id is not None else "all"
    cache_key = (
        f"reports:top-selling:"
        f"start={date_range.start_date}:end={date_range.end_date}:"
        f"limit={limit}:group={p_group_id}"
    )

    cached_data = get_cache(cache_key)

    if cached_data:
        print("CACHE HIT")
        return cached_data
    
    try:
        db_data = _get_top_selling_products_from_db(
            db=db,
            date_range=date_range,
            limit=limit,
            product_group_id=product_group_id
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
    
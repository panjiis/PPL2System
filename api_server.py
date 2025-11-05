# api_server.py
import types 
from fastapi import FastAPI, Depends, HTTPException, Query
from sqlalchemy.orm import Session
from pydantic import BaseModel

from core_logic.sales_logic import *
from core_logic.employee_logic import *
from core_logic.customer_logic import *
from core_logic.dashboard_logic import *

from repositories.db_connection import (
    get_analytics_db, 
    get_pos_db,
    get_inventory_db,
    get_commissions_db
)

app = FastAPI(
    title="Analytics Service API",
)

@app.get("/")
def read_root():
    return {"service": "Analytics Service", "status": "Running"}

# --- Sales Analytics ---
@app.get("/api/v1/reports/sales")
async def http_get_sales_report(
    start_date: str = Query(..., description="Format YYYY-MM-DD"),
    end_date: str = Query(..., description="Format YYYY-MM-DD"),
    
    cashier_id: int | None = Query(None, description="Filter by Cashier ID"),
    product_group_id: int | None = Query(None, description="Filter by Product Group ID"),
    
    include_daily_breakdown: bool = Query(False, description="Include daily breakdown"),
    include_product_breakdown: bool = Query(False, description="Include product breakdown"),
    
    db: Session = Depends(get_analytics_db)
):
    date_range_obj = types.SimpleNamespace()
    date_range_obj.start_date = start_date
    date_range_obj.end_date = end_date

    try:
        report_data = generate_sales_report_logic(
            db=db,
            date_range=date_range_obj,
            cashier_id=cashier_id,
            product_group_id=product_group_id,
            include_daily_breakdown=include_daily_breakdown,
            include_product_breakdown=include_product_breakdown
        )
        
        return report_data
        
    except ValueError as ve:
        raise HTTPException(status_code=400, detail=str(ve))
    except Exception as e:
        print(f"Error in HTTP GetSalesReport: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")

@app.get("/api/v1/reports/daily-summary")
async def http_get_daily_summary(
    date: str = Query(..., description="Format YYYY-MM-DD"),
    cashier_id: int | None = Query(None, description="Filter by Cashier ID"),
    db: Session = Depends(get_analytics_db)
):
    try:
        summaries = get_daily_summary_logic(
            db=db,
            date_str=date,
            cashier_id=cashier_id
        )
        
        return {"daily_summaries": summaries}
        
    except ValueError as ve:
        raise HTTPException(status_code=400, detail=str(ve))
    except Exception as e:
        print(f"Error in HTTP GetDailySummary: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")
    
class GenerateSummaryBody(BaseModel):
    date: str
    cashier_id: int | None = None

@app.post("/api/v1/reports/daily-summary/generate")
async def http_generate_daily_summary(
    body: GenerateSummaryBody, 
    
    analytics_db: Session = Depends(get_analytics_db),
    pos_db: Session = Depends(get_pos_db)
):
    try:
        generated_data = generate_daily_summary_logic(
            analytics_db=analytics_db,
            pos_db=pos_db,
            date_str=body.date,
            cashier_id=body.cashier_id
        )
        
        return {
            "success": True,
            "message": f"Daily summary for {body.date} generated successfully.",
            "generated_summaries": generated_data
        }
        
    except ValueError as ve:
        raise HTTPException(status_code=400, detail=str(ve))
    except Exception as e:
        print(f"Error in HTTP GenerateDailySummary: {e}")
        raise HTTPException(status_code=500, detail=f"Internal server error")

@app.get("/api/v1/products/sales")
async def http_get_product_sales(
    start_date: str = Query(..., description="Format: YYYY-MM-DD"),
    end_date: str = Query(..., description="Format: YYYY-MM-DD"),

    product_id: int | None = Query(None, description="Filter by Product ID"),
    product_group_id: int | None = Query(None, description="Filter by Product Group ID"),

    page_size: int = Query(20, description="Number of records per page"),
    page_token: str | None = Query(None, description="Pagination token"),

    db: Session = Depends(get_analytics_db)
):
    date_range_obj = types.SimpleNamespace(start_date=start_date, end_date=end_date)
    pagination_obj = types.SimpleNamespace(page_size=page_size, page_token=page_token)

    try:
        result_dict = get_product_sales_logic(
            db=db,
            date_range=date_range_obj,
            product_id=product_id,
            product_group_id=product_group_id,
            pagination=pagination_obj
        )

        return {
            "product_sales": result_dict['data'],
            "pagination": {
                "next_page_token": result_dict['next_page_token'],
                "total_count": result_dict['total_count']
            }
        }
    
    except ValueError as ve:
        raise HTTPException(status_code=400, detail=str(ve))
    except Exception as e:
        print(f"Error in HTTP GetProductSales: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")

@app.get("/api/v1/products/top-selling")
async def http_get_top_selling_products(
    start_date: str = Query(..., description="Format: YYYY-MM-DD"),
    end_date: str = Query(..., description="Format: YYYY-MM-DD"),

    limit: int = Query(5, description="Number of top products to return"),

    product_group_id: int | None = Query(None, description="Filter by Product Group ID"),

    db: Session = Depends(get_analytics_db)
):
    date_range_obj = types.SimpleNamespace(start_date=start_date, end_date=end_date)

    try:
        results= get_top_selling_products_logic(
            db=db,
            date_range=date_range_obj,
            limit=limit,
            product_group_id=product_group_id
        )

        return {
            "top_selling_products": results
        }
    
    except ValueError as ve:
        raise HTTPException(status_code=400, detail=str(ve))
    except Exception as e:
        print(f"Error in HTTP GetTopSellingProducts: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")

@app.get("/api/v1/employees/performance")
async def http_get_employee_performance(
    start_date: str = Query(..., description="Format: YYYY-MM-DD"),
    end_date: str = Query(..., description="Format: YYYY-MM-DD"),

    employee_id: int | None = Query(None, description="Filter by Employee ID"),

    page_size: int = Query(20, description="Number of records per page"),
    page_token: str | None = Query(None, description="Pagination token"),

    db: Session = Depends(get_analytics_db)
):
    date_range_obj = types.SimpleNamespace(start_date=start_date, end_date=end_date)
    pagination_obj = types.SimpleNamespace(page_size=page_size, page_token=page_token)

    try:
        result_dict = get_employee_performance_logic(
            db=db,
            date_range=date_range_obj,
            employee_id=employee_id,
            pagination=pagination_obj
        )

        return {
            "performances": result_dict['data'],
            "pagination": {
                "next_page_token": result_dict['next_page_token'],
                "total_count": result_dict['total_count']
            }
        }

    except ValueError as ve:
        raise HTTPException(status_code=400, detail=str(ve))
    except Exception as e:
        print(f"Error in HTTP GetEmployeePerformance: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")
    
@app.get("/api/v1/performance/report")
async def http_get_performance_report(
    start_date: str = Query(..., description="Format: YYYY-MM-DD"),
    end_date: str = Query(..., description="Format: YYYY-MM-DD"),

    employee_id: int | None = Query(None, description="Filter by Employee ID"),

    db: Session = Depends(get_analytics_db)
):
    date_range_obj = types.SimpleNamespace(start_date=start_date, end_date=end_date)

    try:
        result_dict = get_performance_report_logic(
            db=db,
            date_range=date_range_obj,
            employee_id=employee_id
        )

        return {
            "performance_report": result_dict
        }
    
    except ValueError as ve:
        raise HTTPException(status_code=400, detail=str(ve))
    except Exception as e:
        print(f"Error in HTTP GetPerformanceReport: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")
    
@app.get("/api/v1/customers/analytics")
async def http_get_customer_analytics(
    start_date: str = Query(..., description="Format: YYYY-MM-DD"),
    end_date: str = Query(..., description="Format: YYYY-MM-DD"),

    product_group_id: int | None = Query(None, description="Filter by Product Group ID"),

    page_size: int = Query(20, description="Number of records per page"),
    page_token: str | None = Query(None, description="Pagination token"),

    db: Session = Depends(get_analytics_db)
):
    date_range_obj = types.SimpleNamespace(start_date=start_date, end_date=end_date)
    pagination_obj = types.SimpleNamespace(page_size=page_size, page_token=page_token)

    try:
        result_dict = get_customer_analytics_logic(
            db=db,
            date_range=date_range_obj,
            product_group_id=product_group_id,
            pagination=pagination_obj
        )

        return {
            "analytics": result_dict['data'],
            "pagination": {
                "next_page_token": result_dict['next_page_token'],
                "total_count": result_dict['total_count']
            }
        }
    
    except ValueError as ve:
        raise HTTPException(status_code=400, detail=str(ve))
    except Exception as e:
        print(f"Error in HTTP GetCustomerAnalytics: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")
    
@app.get("/api/v1/customers/peak-hours")
async def http_get_peak_hours(
    start_date: str = Query(..., description="Format: YYYY-MM-DD"),
    end_date: str = Query(..., description="Format: YYYY-MM-DD"),

    db: Session = Depends(get_pos_db)
): 
    date_range_obj = types.SimpleNamespace(start_date=start_date, end_date=end_date)

    try:
        results = get_peak_hours_logic(
            db=db,
            date_range=date_range_obj
        )

        return {
            "peak_hours": results
        }

    except ValueError as ve:
        raise HTTPException(status_code=400, detail=str(ve))
    except Exception as e:
        print(f"Error in HTTP GetPeakHours: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")

@app.get("/api/v1/dashboard")
async def http_get_dashboard_data(
    date: str = Query(..., description="Format: YYYY-MM-DD"),

    analytics_db: Session = Depends(get_analytics_db),
    inventory_db: Session = Depends(get_inventory_db),
    commissions_db: Session = Depends(get_commissions_db)
):
    try:
        result_dict = get_dashboard_data_logic(
            analytics_db=analytics_db,
            inventory_db=inventory_db,
            commissions_db=commissions_db,
            date_str=date
        )

        return {
            "dashboard_data": result_dict
        }

    except ValueError as ve:
        raise HTTPException(status_code=400, detail=str(ve))
    except Exception as e:
        print(f"Error in HTTP GetDashboardData: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")

@app.get("/api/v1/real-time-metrics")
async def http_get_real_time_metrics(
    db: Session = Depends(get_pos_db)
):
    try:
        metrics_dict = get_real_time_metrics_logic(db)

        return {
            "metrics": {
                "last_updated": metrics_dict['last_updated'].isoformat(),
                "active_transactions": metrics_dict['active_transactions'],
                "hourly_revenue": metrics_dict['hourly_revenue'],
                "hourly_transaction_count": metrics_dict['hourly_transaction_count'],
                "average_transaction_value": metrics_dict['average_transaction_value'],
                "recent_large_transactions": metrics_dict['recent_large_transactions']
            }
        }
    
    except Exception as e:
        print(f"Error in HTTP GetRealTimeMetrics: {e}")
        raise HTTPException(status_code=500, detail=f"Internal Server Error")
# grpc_server.py
import grpc
from concurrent import futures

import analytics.analytics_service_pb2 as pb
import analytics.analytics_service_pb2_grpc as rpc

from core_logic.sales_logic import *
from core_logic.employee_logic import *
from core_logic.customer_logic import *
from core_logic.dashboard_logic import *
from core_logic.utils import *
from repositories.db_connection import *
from google.protobuf.timestamp_pb2 import Timestamp

def to_proto_timestamp(dt) -> Timestamp:
    if not dt:
        return None
    ts = Timestamp()
    ts.FromDatetime(dt)
    return ts

server_instance = None
GRPC_PORT = 50055

class AnalyticsService(rpc.AnalyticsService):
    # --- Sales Analytics ---
    def GetSalesReport(self, request, context):
        db = get_analytics_db_session()
        
        try:
            report_dict = generate_sales_report_logic(
                db=db,
                date_range=request.date_range,
                cashier_id=request.cashier_id if request.HasField('cashier_id') else None,
                product_group_id=request.product_group_id if request.HasField('product_group_id') else None,
                include_daily_breakdown=request.include_daily_breakdown,
                include_product_breakdown=request.include_product_breakdown
            )

            sales_report_pb = pb.SalesReport(
                period=pb.DateRange(
                    start_date=report_dict["period"]["start_date"],
                    end_date=report_dict["period"]["end_date"]
                ),
                total_revenue=report_dict["summary"]["total_revenue"],
                total_cost=report_dict["summary"]["total_cost"],
                gross_profit=report_dict["summary"]["gross_profit"],
                profit_margin_percentage=report_dict["summary"]["profit_margin_percentage"],
                total_transactions=report_dict["summary"]["total_transactions"],
                total_items_sold=report_dict["summary"]["total_items_sold"],
                average_transaction_value=report_dict["summary"]["average_transaction_value"]
            )
            
            for item in report_dict["daily_breakdowns"]:
                sales_report_pb.daily_breakdowns.append(
                    pb.SalesSummaryDaily(
                        id=item['id'],
                        date=item['date'].isoformat(),
                        cashier_id=item['cashier_id'],
                        total_transactions=item['total_transactions'],
                        total_items_sold=item['total_items_sold'],
                        gross_sales=to_string(item['gross_sales']),
                        total_discounts=to_string(item['total_discounts']),
                        net_sales=to_string(item['net_sales']),
                        total_tax=to_string(item['total_tax']),
                        total_cost=to_string(item['total_cost']),
                        gross_profit=to_string(item['gross_profit']),
                        created_at=to_proto_timestamp(item['created_at']),
                        updated_at=to_proto_timestamp(item['updated_at'])
                    )
                )
                
            for item in report_dict["top_products"]:
                sales_report_pb.top_products.append(
                    pb.ProductSalesSummary(
                        id=0, 
                        date="", 
                        product_id=item['product_id'],
                        product_group_id=item['product_group_id'],
                        quantity_sold=int(item['total_quantity_sold']), 
                        gross_sales=to_string(item['total_gross_sales']),
                        total_discounts=to_string(item['total_discounts_given']),
                        net_sales=to_string(item['total_net_sales']), 
                        total_cost=to_string(item['total_cost_of_goods']),
                        gross_profit=to_string(item['total_gross_profit']),
                        created_at=None,
                        updated_at=None
                    )
                )
            return pb.GetSalesReportResponse(sales_report=sales_report_pb)
            
        except ValueError as ve:
            print(f"Error input: {ve}")
            context.set_code(grpc.StatusCode.INVALID_ARGUMENT)
            context.set_details(str(ve))
            return pb.GetSalesReportResponse()
        except Exception as e:
            print(f"Error in GetSalesReport gRPC: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(f'Internal error: {e}')
            return pb.GetSalesReportResponse()
        finally:
            db.close()

    def GetDailySummary(self, request, context):
        db = get_analytics_db_session()
        try:
            list_of_summaries = get_daily_summary_logic(
                db=db,
                date_str=request.date,
                cashier_id=request.cashier_id if request.HasField('cashier_id') else None
            )

            response = pb.GetDailySummaryResponse()
            
            for item in list_of_summaries:
                summary_pb = pb.SalesSummaryDaily(
                    id=item['id'],
                    date=item['date'], 
                    cashier_id=item['cashier_id'],
                    total_transactions=item['total_transactions'],
                    total_items_sold=item['total_items_sold'],
                    gross_sales=item['gross_sales'], 
                    total_discounts=item['total_discounts'], 
                    net_sales=item['net_sales'], 
                    total_tax=item['total_tax'], 
                    total_cost=item['total_cost'], 
                    gross_profit=item['gross_profit'], 
                    created_at=to_proto_timestamp(item['created_at']), 
                    updated_at=to_proto_timestamp(item['updated_at'])  
                )
                response.daily_summaries.append(summary_pb)
            return response

        except ValueError as ve:
            print(f"Error input in GetDailySummary gRPC: {ve}")
            context.set_code(grpc.StatusCode.INVALID_ARGUMENT)
            context.set_details(str(ve))
            return pb.GetDailySummaryResponse()
        except Exception as e:
            print(f"Error in GetDailySummary gRPC: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(f'Internal error: {e}')
            return pb.GetDailySummaryResponse()
        finally:
            db.close()
        
    def GenerateDailySummary(self, request, context):
        analytics_db = None
        pos_db = None
        
        try:
            analytics_db = get_analytics_db_session()
            pos_db = get_pos_db_session()

            formatted_summaries = generate_daily_summary_logic(
                analytics_db=analytics_db,
                pos_db=pos_db,
                date_str=request.date,
                cashier_id=request.cashier_id if request.HasField('cashier_id') else None
            )
            
            response = pb.GenerateDailySummaryResponse(
                success=True,
                message=f"Daily summary for {request.date} generated successfully."
            )
            
            for item in formatted_summaries:
                response.generated_summaries.append(
                    pb.SalesSummaryDaily(
                        id=item['id'],
                        date=item['date'], 
                        cashier_id=item['cashier_id'],
                        total_transactions=item['total_transactions'],
                        total_items_sold=item['total_items_sold'],
                        gross_sales=item['gross_sales'],       
                        total_discounts=item['total_discounts'], 
                        net_sales=item['net_sales'],         
                        total_tax=item['total_tax'],         
                        total_cost=item['total_cost'],       
                        gross_profit=item['gross_profit'],     
                        created_at=to_proto_timestamp(item['created_at']),
                        updated_at=to_proto_timestamp(item['updated_at'])
                    )
                )
            
            return response

        except ValueError as ve:
            print(f"Error input in GenerateDailySummary gRPC: {ve}")
            context.set_code(grpc.StatusCode.INVALID_ARGUMENT)
            context.set_details(str(ve))
            return pb.GenerateDailySummaryResponse(
                success=False, 
                message=f"Error input: {ve}"
            )
        except Exception as e:
            print(f"Error in GenerateDailySummary gRPC: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(f'Internal error: {e}')
            return pb.GenerateDailySummaryResponse(
                success=False, 
                message=f"Internal error: {e}"
            )
        finally:
            if analytics_db:
                analytics_db.close()
            if pos_db:
                pos_db.close()

    # --- Product Analytics ---
    def GetProductSales(self, request, context):
        db = get_analytics_db_session()
        try:
            result_dict = get_daily_summary_logic(
                db=db,
                date_range=request.date_range,
                product_id=request.product_id if request.HasField('product_id') else None,
                product_group_id=request.product_group_id if request.HasField('product_group_id') else None,
            )

            response = pb.GetProductSalesResponse()

            response.pagination.total_count = result_dict['total_count']
            response.pagination.next_page_token = result_dict['next_page_token']

            for item in result_dict['data']:
                response.product_sales.append(
                    pb.ProductSalesSummary(
                        id=item['id'],
                        date=item['date'], 
                        product_id=item['product_id'],
                        product_group_id=item['product_group_id'],
                        quantity_sold=item['quantity_sold'],
                        gross_sales=item['gross_sales'],      
                        total_discounts=item['total_discounts'],
                        net_sales=item['net_sales'],        
                        total_cost=item['total_cost'],      
                        gross_profit=item['gross_profit'],    
                        created_at=to_proto_timestamp(item['created_at']),
                        updated_at=to_proto_timestamp(item['updated_at'])
                    )
                )
            return response

        except ValueError as ve:
            print(f"Input error in GetProductSales gRPC: {ve}")
            context.set_details(str(ve))
            return pb.GetProductSalesResponse()
        except Exception as e:
            print(f"Error in GetProductSales gRPC: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(f'Internal error: {e}')
            return pb.GetProductSalesResponse()
        finally:
            if db:
                db.close()
    
    def GetTopSellingProducts(self, request, context):
        db = get_analytics_db_session()
        try:
            formatted_products = get_top_selling_products_logic(
                db=db,
                date_range=request.date_range,
                limit=request.limit,
                product_id=request.product_id if request.HasField('product_id') else None,
            )

            response = pb.GetTopSellingProductsResponse()

            for item in formatted_products:
                response.top_products.append(
                    pb.ProductSalesSummary(
                        id=0, 
                        date="", 
                        
                        product_id=item['product_id'],
                        product_group_id=item['product_group_id'],
                        quantity_sold=item['quantity_sold'],
                        gross_sales=item['gross_sales'],
                        total_discounts=item['total_discounts'],
                        net_sales=item['net_sales'],
                        total_cost=item['total_cost'],
                        gross_profit=item['gross_profit'],
                        
                        created_at=None,
                        updated_at=None
                    )
                )
            return response
        
        except ValueError as ve:
            print(f"Input error in GetTopSellingProducts gRPC: {ve}")
            context.set_code(grpc.StatusCode.INVALID_ARGUMENT)
            context.set_details(str(ve))
            return pb.GetTopSellingProductsResponse()
        except Exception as e:
            print(f"Error in GetTopSellingProducts gRPC: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(f'Internal error: {e}')
            return pb.GetTopSellingProductsResponse()
        finally:
            if db:
                db.close()
    
    def GetEmployeePerformance(self, request, context):
        db = get_analytics_db_session()

        try:
            result_dict = get_employee_performance_logic(
                db=db,
                date_range=request.date_range,
                employee_id=request.employee_id if request.HasField('employee_id') else None,
                pagination=request.pagination
            )

            response = pb.GetEmployeePerformanceResponse()

            response.pagination.total_count = result_dict['total_count']
            response.pagination.next_page_token = result_dict['next_page_token']

            for item in result_dict['data']:
                response.performances.append(
                    pb.EmployeePerformance(
                        id=item['id'],
                        date=item['date'], 
                        employee_id=item['employee_id'],
                        total_sales=item['total_sales'],           
                        total_transactions=item['total_transactions'],
                        total_items_sold=item['total_items_sold'],
                        commission_earned=item['commission_earned'], 
                        performance_score=item['performance_score'], 
                        created_at=to_proto_timestamp(item['created_at']),
                        updated_at=to_proto_timestamp(item['updated_at'])
                    )
                )
        
            return response
        
        except ValueError as ve:
            print(f"Input error in GetEmployeePerformance gRPC: {ve}")
            context.set_code(grpc.StatusCode.INVALID_ARGUMENT)
            context.set_details(str(ve))
            return pb.GetEmployeePerformanceResponse()
        except Exception as e:
            print(f"Error in GetEmployeePerformance gRPC: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(f'Internal error: {e}')
            return pb.GetEmployeePerformanceResponse()
        finally:
            if db:
                db.close()

    def GetPerformanceReport(self, request, context):
        db = get_analytics_db_session()

        try:
            report_dict = get_performance_report_logic(
                db=db,
                date_range=request.date_range,
                employee_id=request.employee_id if request.HasField('employee_id') else None
            )

            response = pb.GetPerformanceReportResponse()

            report_pb = pb.PeformanceReport(
                period=pb.DateRange(
                    start_date=report_dict["period"]["start_date"],
                    end_date=report_dict["period"]["end_date"]
                ),
                total_commissions=report_dict["total_commissions"],
                top_performer_employee_id=report_dict["top_performer_employee_id"],
                top_performer_sales=report_dict["top_performer_sales"]
            )

            for item in report_dict["employee_performances"]:
                report_pb.employee_performances.append(
                    pb.EmployeePerformance(
                        id=item['id'],
                        date=item['date'],
                        employee_id=item['employee_id'],
                        total_sales=item['total_sales'],
                        total_transactions=item['total_transactions'],
                        total_items_sold=item['total_items_sold'],
                        commission_earned=item['commission_earned'],
                        performance_score=item['performance_score'],
                        created_at=to_proto_timestamp(item['created_at']),
                        updated_at=to_proto_timestamp(item['updated_at'])
                    )
                )
            
            response.performance_report.CopyFrom(report_pb)
            return response
        
        except ValueError as ve:
            print(f"Input error in GetPerformanceReport gRPC: {ve}")
            context.set_code(grpc.StatusCode.INVALID_ARGUMENT)
            context.set_details(str(ve))
            return pb.GetPerformanceReportResponse()
        except Exception as e:
            print(f"Error in GetPerformanceReport gRPC: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(f'Internal error: {e}')
            return pb.GetPerformanceReportResponse()
        finally:
            if db:
                db.close()

    def GetCustomerAnalytics(self, request, context):
        db = get_analytics_db_session()

        try:
            result_dict = get_customer_analytics_logic(
                db=db,
                date_range=request.date_range,
                product_group_id=request.product_group_id if request.HasField('product_group_id') else None,
                pagination=request.pagination
            )

            response = pb.GetCustomerAnalyticsResponse()

            response.pagination.total_count = result_dict['total_count']
            response.pagination.next_page_token = result_dict['next_page_token']

            for item in result_dict['data']:
                response.analytics.append(
                    pb.CustomerAnalytics(
                        id=item['id'],
                        date=item['date'], 
                        product_group_id=item['product_group_id'],
                        total_transactions=item['total_transactions'],
                        total_revenue=item['total_revenue'],                 
                        average_transaction_value=item['average_transaction_value'], 
                        peak_hour=item['peak_hour'],
                        created_at=to_proto_timestamp(item['created_at']),
                        updated_at=to_proto_timestamp(item['updated_at'])
                    )
                )
            
            return response
        
        except ValueError as ve:
            print(f"Input error in GetCustomerAnalytics gRPC: {ve}")
            context.set_code(grpc.StatusCode.INVALID_ARGUMENT)
            context.set_details(str(ve))
            return pb.GetCustomerAnalyticsResponse()
        except Exception as e:
            print(f"Error in GetCustomerAnalytics gRPC: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(f'Internal error: {e}')
            return pb.GetCustomerAnalyticsResponse()
        finally:
            if db:
                db.close()

    def GetPeakHours(self, request, context):
        db = get_pos_db_session()
        
        try:
            formatted_data = get_peak_hours_logic(
                db=db,
                date_range=request.date_range
            )

            response = pb.GetPeakHoursResponse()

            for item in formatted_data:
                response.peak_hours.append(
                    pb.PeakHourData(
                        hour=item['hour'],
                        transaction_count=item['transaction_count'],
                        total_revenue=item['total_revenue']
                    )
                )
            
            return response
        
        except ValueError as ve:
            print(f"Input error in GetPeakHours gRPC: {ve}")
            context.set_code(grpc.StatusCode.INVALID_ARGUMENT)
            context.set_details(str(ve))
            return pb.GetPeakHoursResponse()
        except Exception as e:
            print(f"Error in GetPeakHours gRPC: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(f'Internal error: {e}')
            return pb.GetPeakHoursResponse()
        finally:
            if db:
                db.close()
    
    def GetDashboardData(self, request, context):
        analytics_db = get_analytics_db_session()
        inventory_db = get_inventory_db_session()
        commissions_db = get_commissions_db_session()

        try:
            result_dict = get_dashboard_data_logic(
                analytics_db=analytics_db,
                inventory_db=inventory_db,
                commissions_db=commissions_db,
                date_str=request.date
            )

            dashboard_pb = pb.DashboardData(
                today_revenue=result_dict['today_revenue'],
                today_transactions=result_dict['today_transactions'],
                today_items_sold=result_dict['today_items_sold'],
                today_profit=result_dict['today_profit'],
                revenue_change_percentage=result_dict['revenue_change_percentage'],
                transaction_change_percentage=result_dict['transaction_change_percentage'],
                pending_commissions_count=result_dict['pending_commissions_count']
            )

            dashboard_pb.low_stock_alerts.extend(result_dict['low_stock_alerts'])

            for item in result_dict['top_products_today']:
                dashboard_pb.top_products_today.append(
                    pb.EmployeePerformance(
                        id=0, date="",
                        employee_id=item['employee_id'],
                        total_sales=to_string(item['total_sales']),
                    )
                )
            
            return pb.GetDashboardDataResponse(dashboard_data=dashboard_pb)
        
        except ValueError as ve:
            print(f"Input error in GetDashboardData gRPC: {ve}")
            context.set_code(grpc.StatusCode.INVALID_ARGUMENT)
            context.set_details(str(ve))
            return pb.GetDashboardDataResponse()
        except Exception as e:
            print(f"Error in GetDashboardData gRPC: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(f'Internal error: {e}')
            return pb.GetDashboardDataResponse()
        finally:
            if analytics_db:
                analytics_db.close()
            if inventory_db:
                inventory_db.close()
            if commissions_db:
                commissions_db.close()
    
    def GetRealTimeMetrics(self, request, context):
        db = get_pos_db_session()
    
        try:
            metrics_dict = get_real_time_metrics_logic(db)

            response = pb.GetRealTimeMetricsResponse()

            metrics_pb = pb.RealTimeMetrics(
                last_updated=to_proto_timestamp(metrics_dict['last_updated']),
                active_transactions=metrics_dict['active_transactions'],
                hourly_revenue=metrics_dict['hourly_revenue'],
                hourly_transaction_count=metrics_dict['hourly_transaction_count'],
                average_transaction_value=metrics_dict['average_transaction_value'],
                recent_large_transactions=metrics_dict['recent_large_transactions']
            )

            response.metrics.CopyFrom(metrics_pb)

            return response
        
        except Exception as e:
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(f"Internal error: {e}")
            return pb.GetRealTimeMetricsResponse()
        finally:
            if db:
                db.close()

def start_grpc_server():
    global server_instance
    print(f"Memulai server gRPC di port {GRPC_PORT}...")

    server_instance = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    rpc.add_AnalyticsServiceServicer_to_server(AnalyticsService(), server_instance)
    server_instance.add_insecure_port(f'[::]:{GRPC_PORT}')

    server_instance.start()
    print("gRPC server started")

def stop_grpc_server():
    global server_instance
    if server_instance:
        print("Stopping gRPC server...")
        server_instance.stop(0)
        server_instance = None
        print("gRPC server stopped")

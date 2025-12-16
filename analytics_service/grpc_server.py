# grpc_server.py
import grpc
from concurrent import futures
import asyncio
import threading
from dateutil import parser

import analytics.analytics_service_pb2 as pb
import analytics.analytics_service_pb2_grpc as rpc

from core_logic.sales_logic import *
from core_logic.employee_logic import *
from core_logic.customer_logic import *
from core_logic.dashboard_logic import *
from core_logic.utils import *
from repositories.db_connection import *
from google.protobuf.timestamp_pb2 import Timestamp

from pubsub_subscriber import run_subscriber

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
                
            for item in report_dict["top_products"]:
                sales_report_pb.top_products.append(
                    pb.ProductSalesSummary(
                        id=0, 
                        date="", 
                        product_code=item['product_code'],
                        product_group_id=item['product_group_id'],
                        quantity_sold=int(item['total_quantity_sold']), 
                        gross_sales=item['total_gross_sales'],
                        total_discounts=item['total_discounts_given'],
                        net_sales=item['total_net_sales'], 
                        total_cost=item['total_cost_of_goods'],
                        gross_profit=item['total_gross_profit'],
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
                created_at_dt = parser.isoparse(item['created_at']) if isinstance(item['created_at'], str) else item['created_at']
                updated_at_dt = parser.isoparse(item['updated_at']) if isinstance(item['updated_at'], str) else item['updated_at']

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
                    created_at=to_proto_timestamp(created_at_dt), 
                    updated_at=to_proto_timestamp(updated_at_dt)  
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
        db = None
        
        try:
            db = get_analytics_db_session()

            formatted_summaries = generate_daily_summary_logic(
                db=db,
                date_str=request.date,
                cashier_id=request.cashier_id if request.HasField('cashier_id') else None
            )
            
            response = pb.GenerateDailySummaryResponse(
                success=True,
                message=f"Daily summary for {request.date} generated successfully."
            )
            
            for item in formatted_summaries:
                created_at_dt = parser.isoparse(item['created_at']) if isinstance(item['created_at'], str) else item['created_at']
                updated_at_dt = parser.isoparse(item['updated_at']) if isinstance(item['updated_at'], str) else item['updated_at']

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
                        created_at=to_proto_timestamp(created_at_dt), # <-- Gunakan variabel baru
                        updated_at=to_proto_timestamp(updated_at_dt)  # <-- Gunakan variabel baru
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
            if db:
                db.close()

    # --- Product Analytics ---
    def GetProductSales(self, request, context):
        db = get_analytics_db_session()
        try:
            result_dict = get_product_sales_logic(  
                db=db,
                date_range=request.date_range,
                product_code=request.product_code if request.HasField('product_code') else None,
                product_group_id=request.product_group_id if request.HasField('product_group_id') else None,
                pagination=request.pagination  
            )

            response = pb.GetProductSalesResponse()

            response.pagination.total_count = result_dict['total_count']
            response.pagination.next_page_token = result_dict['next_page_token']

            for item in result_dict['data']:
                response.product_sales.append(
                    pb.ProductSalesSummary(
                        id=item['id'],
                        date=item['date'], 
                        product_code=item['product_code'],
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
            # print(f"response : \n{response}")
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
                product_group_id=request.product_group_id if request.HasField('product_group_id') else None,
                # product_code=request.product_code if request.HasField('product_code') else None,
            )

            response = pb.GetTopSellingProductsResponse()

            for item in formatted_products:
                response.top_products.append(
                    pb.ProductSalesSummary(
                        id=0, 
                        date="", 
                        
                        product_code=item['product_code'],
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
                created_at_dt = parser.isoparse(item['created_at']) if isinstance(item['created_at'], str) else item['created_at']
                updated_at_dt = parser.isoparse(item['updated_at']) if isinstance(item['updated_at'], str) else item['updated_at']
                
                period_start_str = item['period_start'].isoformat() if isinstance(item['period_start'], datetime.date) else item['period_start']
                period_end_str = item['period_end'].isoformat() if isinstance(item['period_end'], datetime.date) else item['period_end']

                response.performances.append(
                    pb.EmployeePerformance(
                        id=item['id'],
                        calculation_id=item['calculation_id'],  
                        employee_id=item['employee_id'],
                        period_start=period_start_str,
                        period_end=period_end_str,
                        total_sales=item['total_sales'],           
                        total_transactions=item['total_transactions'],
                        total_items_sold=item['total_items_sold'],
                        commission_earned=item['commission_earned'], 
                        # performance_score=item['performance_score'], 
                        created_at=to_proto_timestamp(created_at_dt),
                        updated_at=to_proto_timestamp(updated_at_dt)
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

            report_pb = pb.PerformanceReport(
                period=pb.DateRange(
                    start_date=report_dict.get("period", {}).get("start_date", ""),
                    end_date=report_dict.get("period", {}).get("end_date", "")
                ),
                total_commissions=report_dict.get("total_commissions", "0"),
                top_performer_employee_id=report_dict.get("top_performer_employee_id", 0),
                top_performer_sales=report_dict.get("top_performer_sales", "0") 
            )

            for item in report_dict["employee_performances"]:
                created_at_dt = parser.isoparse(item['created_at']) if isinstance(item['created_at'], str) else item['created_at']
                updated_at_dt = parser.isoparse(item['updated_at']) if isinstance(item['updated_at'], str) else item['updated_at']
                
                period_start_str = item['period_start'].isoformat() if isinstance(item['period_start'], datetime.date) else item['period_start']
                period_end_str = item['period_end'].isoformat() if isinstance(item['period_end'], datetime.date) else item['period_end']

                report_pb.employee_performances.append(
                    pb.EmployeePerformance(
                        id=item['id'],
                        # date=item['date'],
                        calculation_id=item['calculation_id'],
                        employee_id=item['employee_id'],
                        period_start=period_start_str,
                        period_end=period_end_str,
                        total_sales=item['total_sales'],
                        total_transactions=item['total_transactions'],
                        total_items_sold=item['total_items_sold'],
                        commission_earned=item['commission_earned'],
                        # performance_score=item['performance_score'],
                        created_at=to_proto_timestamp(created_at_dt),
                        updated_at=to_proto_timestamp(updated_at_dt)
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
                        # peak_hour=item['peak_hour'],
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
        db = get_analytics_db_session()
        
        try:
            weekly_data_list = get_weekly_peak_hours_logic(
                db=db,
            )

            response = pb.GetPeakHoursResponse()

            for week_day_data in weekly_data_list:
                # 1. Buat data hari (WeeklyPeakData)
                pb_week_day = pb.WeeklyPeakData(
                    day_of_week=week_day_data['day_of_week']
                )
                
                # 2. Isi data per jam (HourlyData)
                for hour_data in week_day_data['hourly_data']:
                    pb_hour = pb.HourlyData(
                        hour=hour_data['hour'],
                        transaction_count=hour_data['transaction_count'],
                        total_revenue=hour_data['total_revenue']
                    )
                    pb_week_day.hourly_data.append(pb_hour)
                
                # 3. Tambahkan data hari yang sudah lengkap ke respons
                response.peak_data_by_week.append(pb_week_day)
            
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
        # inventory_db = get_inventory_db_session()
        # commissions_db = get_commissions_db_session()

        try:
            result_dict = get_dashboard_data_logic(
                analytics_db=analytics_db,
                # inventory_db=inventory_db,
                # commissions_db=commissions_db,
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

            # dashboard_pb.low_stock_alerts.extend(result_dict['low_stock_alerts'])

            for item in result_dict['low_stock_alerts']:
                dashboard_pb.low_stock_alerts.append(
                    pb.LowStockItem(
                        product_name=item['product_name'],
                        remaining_quantity=item['remaining_quantity']
                    )
                )

            # for item in result_dict['top_products_today']:
            #     dashboard_pb.top_products_today.append(
            #         pb.EmployeePerformance(
            #             id=0, date="",
            #             employee_id=item['employee_id'],
            #             total_sales=to_string(item['total_sales']),
            #         )
            #     )
            
            for item in result_dict.get('top_products_today', []):
                dashboard_pb.top_products_today.append(
                    pb.ProductSalesSummary(
                        # Asumsi 'item' memiliki kunci ini, sesuaikan jika perlu
                        product_code=item.get('product_code', 0),
                        net_sales=str(item.get('net_sales', 0)),
                        quantity_sold=int(item.get('quantity_sold', 0))
                    )
                )
            
            for item in result_dict.get('top_performers_today', []):
                dashboard_pb.top_performers_today.append(
                    pb.EmployeePerformance(
                        employee_id=item.get('employee_id', 0),
                        total_sales=str(item.get('total_sales', 0))
                    )
                )
            
            return pb.GetDashboardDataResponse(dashboard=dashboard_pb)
        
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
            # if inventory_db:
            #     inventory_db.close()
            # if commissions_db:
            #     commissions_db.close()
    
    def GetRealTimeMetrics(self, request, context):
        # db = get_pos_db_session()
    
        try:
            # metrics_dict = get_real_time_metrics_logic(db)

            metrics_dict = get_realtime_metrics_from_cache()

            response = pb.GetRealTimeMetricsResponse()

            last_updated_dt = parser.isoparse(metrics_dict['last_updated']) if isinstance(metrics_dict['last_updated'], str) else metrics_dict['last_updated']

            last_updated_ts = to_proto_timestamp(last_updated_dt)

            metrics_pb = pb.RealTimeMetrics(
                last_updated=last_updated_ts,
                # active_transactions=metrics_dict['active_transactions'],
                hourly_revenue=metrics_dict['hourly_revenue'],
                hourly_transaction_count=metrics_dict['hourly_transaction_count'],
                average_transaction_value=metrics_dict['average_transaction_value'],
                # recent_large_transactions=metrics_dict['recent_large_transactions']
            )

            for tx_dict in metrics_dict['recent_large_transactions']:
                metrics_pb.recent_large_transactions.append(
                    pb.LargeTransaction(
                        doc=tx_dict.get("doc"),
                        amount=tx_dict.get("amount")
                    )
                )

            response.metrics.CopyFrom(metrics_pb)

            return response
        
        except Exception as e:
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(f"Internal error: {e}")
            return pb.GetRealTimeMetricsResponse()
        # finally:
        #     if db:
        #         db.close()
    
    def GenerateProductSalesSummary(self, request, context):
        analytics_db = None
        try:
            analytics_db = get_analytics_db_session()

            formatted_summaries = generate_product_sales_summary_logic(
                analytics_db=analytics_db,
                date_str=request.date,
                product_code=request.product_code if request.HasField('product_code') else None,
                product_group_id=request.product_group_id if request.HasField('product_group_id') else None
            )
            
            response = pb.GenerateProductSalesSummaryResponse(
                success=True,
                message=f"Product sales summary for {request.date} generated successfully."
            )

            print(formatted_summaries)
            
            # Ubah format ke Protobuf
            for item in formatted_summaries:
                # Perlu konversi datetime/decimal
                response.generated_summaries.append(
                    pb.ProductSalesSummary(
                        id=0, # ID dari upsert mungkin tidak relevan di sini
                        product_code=item['product_code'],
                        date=item['date'].isoformat(),
                        product_group_id=item['product_group_id'],
                        quantity_sold=int(item['quantity_sold']),
                        gross_sales=to_string(item['gross_sales']),
                        total_discounts=to_string(item['total_discounts']),
                        net_sales=to_string(item['net_sales']),
                        total_cost=to_string(item['total_cost']),
                        gross_profit=to_string(item['gross_profit']),
                        created_at=None, # Kita tidak mengambil ini dari kueri
                        updated_at=None
                    )
                )
            
            return response

        except ValueError as ve:
            context.set_code(grpc.StatusCode.INVALID_ARGUMENT)
            context.set_details(str(ve))
            return pb.GenerateProductSalesSummaryResponse(success=False, message=f"Error input: {ve}")
        except Exception as e:
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(f'Internal error: {e}')
            return pb.GenerateProductSalesSummaryResponse(success=False, message=f"Internal error: {e}")
        finally:
            if analytics_db:
                analytics_db.close()
    
    def GenerateCustomerAnalytics(self, request, context):
        analytics_db = None
        try:
            analytics_db = get_analytics_db_session()

            formatted_analytics = generate_customer_analytics_logic(
                analytics_db=analytics_db,
                date_str=request.date,
                product_group_id=request.product_group_id if request.HasField('product_group_id') else None
            )
            
            response = pb.GenerateCustomerAnalyticsResponse(
                success=True,
                message=f"Customer analytics for {request.date} generated successfully."
            )
            
            # Ubah format ke Protobuf
            for item in formatted_analytics:
                response.generated_analytics.append(
                    pb.CustomerAnalytics(
                        date=item['date'].isoformat(),
                        product_group_id=item['product_group_id'],
                        total_transactions=item['total_transactions'],
                        total_revenue=to_string(item['total_revenue']),
                        average_transaction_value=to_string(item['average_transaction_value']),
                        created_at=None,
                        updated_at=None
                    )
                )
            
            return response

        except ValueError as ve:
            context.set_code(grpc.StatusCode.INVALID_ARGUMENT)
            context.set_details(str(ve))
            return pb.GenerateCustomerAnalyticsResponse(success=False, message=f"Error input: {ve}")
        except Exception as e:
            print(f"Error in GenerateCustomerAnalytics gRPC: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(f'Internal error: {e}')
            return pb.GenerateCustomerAnalyticsResponse(success=False, message=f"Internal error: {e}")
        finally:
            if analytics_db:
                analytics_db.close()
    
    def GenerateEmployeePerformance(self, request, context):
        analytics_db = None
        try:
            analytics_db = get_analytics_db_session()

            # Panggil logika baru dengan calculation_id
            performance_data = generate_employee_performance_logic(
                analytics_db=analytics_db,
                calculation_id=request.calculation_id
            )
            
            response = pb.GenerateEmployeePerformanceResponse(
                success=True,
                message=f"Employee performance for CalculationID {request.calculation_id} generated successfully."
            )
            
            # Ubah format ke Protobuf
            response.generated_performance.CopyFrom(
                pb.EmployeePerformance(
                    # id=0, # ID tidak perlu dikembalikan
                    calculation_id=performance_data['calculation_id'],
                    employee_id=performance_data['employee_id'],
                    period_start=performance_data['period_start'].isoformat(),
                    period_end=performance_data['period_end'].isoformat(),
                    total_sales=to_string(performance_data['total_sales']),
                    total_transactions=int(performance_data['total_transactions']),
                    total_items_sold=int(performance_data['total_items_sold']),
                    commission_earned=to_string(performance_data['commission_earned']),
                    created_at=None,
                    updated_at=None
                )
            )
            
            return response

        except ValueError as ve:
            context.set_code(grpc.StatusCode.INVALID_ARGUMENT)
            context.set_details(str(ve))
            return pb.GenerateEmployeePerformanceResponse(success=False, message=str(ve))
        except Exception as e:
            print(f"Error in GenerateEmployeePerformance gRPC: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(f'Internal error: {e}')
            return pb.GenerateEmployeePerformanceResponse(success=False, message=f"Internal error: {e}")
        finally:
            if analytics_db:
                analytics_db.close()
    
    def ClearLowStockCache(self, request, context):
        print("testpy")
        try:
            success = clear_low_stock_cache_logic()
            
            if success:
                return pb.ClearLowStockCacheResponse(
                    success=True, 
                    message="Low stock cache cleared successfully"
                )
            else:
                return pb.ClearLowStockCacheResponse(
                    success=False, 
                    message="Failed to connect to Redis"
                )
        except Exception as e:
            print(f"Error in ClearLowStockCache: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            return pb.ClearLowStockCacheResponse(success=False, message=str(e))

def start_grpc_server():
    global server_instance
    print(f"Memulai server gRPC di port {GRPC_PORT}...")

    server_instance = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    rpc.add_AnalyticsServiceServicer_to_server(AnalyticsService(), server_instance)
    server_instance.add_insecure_port(f'[::]:{GRPC_PORT}')

    server_instance.start()
    print("gRPC server started")

    # Jalankan NATS subscriber di background thread
    print("Starting NATS Subscriber on background thread...")
    subscriber_thread = threading.Thread(
        target=lambda: asyncio.run(run_subscriber()),
        daemon=True # Pastikan thread ini berhenti saat aplikasi utama berhenti
    )
    subscriber_thread.start()

def stop_grpc_server():
    global server_instance
    if server_instance:
        print("Stopping gRPC server...")
        server_instance.stop(0)
        server_instance = None
        print("gRPC server stopped")

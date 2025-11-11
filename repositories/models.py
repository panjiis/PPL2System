# repositories/models.py
from sqlalchemy import (
    Column, BigInteger, Date, Integer, Numeric, DateTime, String, UniqueConstraint, Index, Identity
)
from sqlalchemy.ext.declarative import declarative_base
from sqlalchemy.sql import func

# Tentukan schema jika Anda menggunakannya, misal: 'public'
# SCHEMA_NAME = 'public' 
Base = declarative_base()
# Jika menggunakan schema:
# Base = declarative_base(metadata=MetaData(schema=SCHEMA_NAME))

class SalesSummaryDaily(Base):
    __tablename__ = 'sales_summary_daily'
    
    # Definisi kolom berdasarkan analytics_syntra_microservice.sql
    id = Column(BigInteger, Identity(always=False), primary_key=True)
    date = Column(Date, nullable=False)
    cashier_id = Column(BigInteger, nullable=False)
    total_transactions = Column(Integer, server_default='0')
    total_items_sold = Column(Integer, server_default='0')
    gross_sales = Column(Numeric(15, 2), server_default='0.00')
    total_discounts = Column(Numeric(15, 2), server_default='0.00')
    net_sales = Column(Numeric(15, 2), server_default='0.00')
    total_tax = Column(Numeric(15, 2), server_default='0.00')
    total_cost = Column(Numeric(15, 2), server_default='0.00')
    gross_profit = Column(Numeric(15, 2), server_default='0.00')
    created_at = Column(DateTime, server_default=func.now())
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now())

    # Definisi unique constraint berdasarkan index di .sql
    __table_args__ = (
        Index('sales_summary_daily_date_cashier_id_idx', 'date', 'cashier_id', unique=True),
    )

class ProductSalesSummary(Base):
    __tablename__ = 'product_sales_summary'
    
    id = Column(BigInteger, Identity(always=False), primary_key=True)
    date = Column(Date, nullable=False)
    product_id = Column(Integer, nullable=False)
    product_group_id = Column(Integer, nullable=True)
    quantity_sold = Column(Integer, server_default='0')
    gross_sales = Column(Numeric(15, 2), server_default='0.00')
    total_discounts = Column(Numeric(15, 2), server_default='0.00')
    net_sales = Column(Numeric(15, 2), server_default='0.00')
    total_cost = Column(Numeric(15, 2), server_default='0.00')
    gross_profit = Column(Numeric(15, 2), server_default='0.00')
    created_at = Column(DateTime, server_default=func.now())
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now())

    __table_args__ = (
        Index('product_sales_summary_date_product_id_idx', 'date', 'product_id', unique=True),
    )

class EmployeePerformance(Base):
    __tablename__ = 'employee_performance'
    
    id = Column(BigInteger, Identity(always=False), primary_key=True)
    date = Column(Date, nullable=False)
    employee_id = Column(BigInteger, nullable=False)
    total_sales = Column(Numeric(15, 2), server_default='0.00')
    total_transactions = Column(Integer, server_default='0')
    total_items_sold = Column(Integer, server_default='0')
    commission_earned = Column(Numeric(15, 2), server_default='0.00')
    performance_score = Column(Numeric(5, 2), server_default='0.00')
    created_at = Column(DateTime, server_default=func.now())
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now())

    __table_args__ = (
        Index('employee_performance_date_employee_id_idx', 'date', 'employee_id', unique=True),
    )

class CustomerAnalytics(Base):
    __tablename__ = 'customer_analytics'
    
    id = Column(BigInteger, Identity(always=False), primary_key=True)
    date = Column(Date, nullable=False)
    product_group_id = Column(Integer, nullable=True)
    total_transactions = Column(Integer, server_default='0')
    total_revenue = Column(Numeric(15, 2), server_default='0.00')
    average_transaction_value = Column(Numeric(15, 2), server_default='0.00')
    peak_hour = Column(String, nullable=True)
    created_at = Column(DateTime, server_default=func.now())
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now())

    __table_args__ = (
        Index('customer_analytics_date_product_group_id_idx', 'date', 'product_group_id', unique=True),
    )

class RawSalesEvent(Base):
    __tablename__ = 'raw_sales_events'
    
    # Ini adalah tabel baru dari diskusi kita sebelumnya
    id = Column(BigInteger, Identity(always=False), primary_key=True)
    order_document_number = Column(String(255), nullable=False, unique=True)
    order_timestamp = Column(DateTime(timezone=True), nullable=False, index=True)
    total_amount = Column(Numeric(18, 2), nullable=False)
    cashier_id = Column(BigInteger, nullable=False)
    processed_at = Column(DateTime(timezone=True), server_default=func.now())

    __table_args__ = (
        Index('idx_raw_sales_events_order_timestamp', 'order_timestamp'),
    )

# ... (model-model lain) ...

class RawOrderDocument(Base):
    __tablename__ = 'raw_order_documents'

    id = Column(BigInteger, Identity(always=False), primary_key=True) # ID dari event, BUKAN auto-increment
    document_number = Column(String(255), unique=True, nullable=False)
    cashier_id = Column(BigInteger, nullable=False)
    order_timestamp = Column(DateTime(timezone=True), nullable=False, index=True)
    tax_amount = Column(Numeric(18, 2), nullable=False, server_default='0.00')
    # Tambahkan field lain dari 'order_documents' jika perlu

class RawOrderItem(Base):
    __tablename__ = 'raw_order_items'

    id = Column(BigInteger, Identity(always=False), primary_key=True)
    document_number = Column(String(255), nullable=False, index=True)
    product_code = Column(String(255), nullable=False)
    quantity = Column(Integer, nullable=False)
    price_before_discount = Column(Numeric(18, 2), nullable=False)
    discount_amount = Column(Numeric(18, 2), nullable=False)
    line_total = Column(Numeric(18, 2), nullable=False)
    cost_price = Column(Numeric(18, 2), nullable=False) # KOLOM KUNCI!
    # Tambahkan field lain dari 'order_items' jika perlu
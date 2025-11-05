import os
import random
import datetime
from decimal import Decimal
from dotenv import load_dotenv
from sqlalchemy import create_engine, text
from faker import Faker

# Impor "penerjemah DSN" kita
try:
    from repositories.db_connection import convert_dsn_to_url
except ImportError:
    print("Error: Tidak dapat mengimpor 'convert_dsn_to_url'.")
    print("Pastikan file 'repositories/db_connection.py' ada dan benar.")
    exit(1)

# --- Konfigurasi Data Palsu ---
DAYS_TO_GENERATE = 30
CASHIER_IDS = [101, 102, 103] # Asumsi 3 kasir
EMPLOYEE_IDS = [101, 102, 103, 201, 202] # Asumsi 5 karyawan
PRODUCT_IDS = [10, 11, 12, 13, 14, 15] # Asumsi 6 produk
GROUP_IDS = [1, 2] # Asumsi 2 grup produk

# Inisialisasi Faker
fake = Faker('id_ID') # Menggunakan lokal Indonesia

def generate_dummy_data():
    print(f"Membuat data palsu untuk {DAYS_TO_GENERATE} hari...")
    
    # Siapkan list untuk menampung data
    sales_data = []
    product_data = []
    employee_data = []
    customer_data = []
    
    # ID Counter (karena ID bukan BIGSERIAL)
    id_counters = {'sales': 1, 'product': 1, 'employee': 1, 'customer': 1}
    
    today = datetime.date.today()
    
    for i in range(DAYS_TO_GENERATE):
        current_date = today - datetime.timedelta(days=i)
        
        # 1. Buat data sales_summary_daily (per kasir per hari)
        for cashier_id in CASHIER_IDS:
            net_sales = Decimal(random.randint(500000, 2000000))
            total_discounts = net_sales / Decimal(random.randint(5, 10))
            gross_sales = net_sales + total_discounts
            total_cost = net_sales * Decimal(random.uniform(0.6, 0.8))
            gross_profit = net_sales - total_cost
            
            sales_data.append({
                "id": id_counters['sales'],
                "date": current_date,
                "cashier_id": cashier_id,
                "total_transactions": random.randint(20, 50),
                "total_items_sold": random.randint(100, 300),
                "gross_sales": gross_sales,
                "total_discounts": total_discounts,
                "net_sales": net_sales,
                "total_tax": net_sales * Decimal('0.11'),
                "total_cost": total_cost,
                "gross_profit": gross_profit
            })
            id_counters['sales'] += 1

        # 2. Buat data product_sales_summary (per produk per hari)
        for product_id in PRODUCT_IDS:
            net_sales = Decimal(random.randint(100000, 500000))
            total_discounts = net_sales / Decimal(random.randint(5, 10))
            gross_sales = net_sales + total_discounts
            total_cost = net_sales * Decimal(random.uniform(0.6, 0.8))
            gross_profit = net_sales - total_cost
            
            product_data.append({
                "id": id_counters['product'],
                "date": current_date,
                "product_id": product_id,
                "product_group_id": random.choice(GROUP_IDS),
                "quantity_sold": random.randint(10, 50),
                "gross_sales": gross_sales,
                "total_discounts": total_discounts,
                "net_sales": net_sales,
                "total_cost": total_cost,
                "gross_profit": gross_profit
            })
            id_counters['product'] += 1
            
        # 3. Buat data employee_performance (per karyawan per hari)
        for employee_id in EMPLOYEE_IDS:
            employee_data.append({
                "id": id_counters['employee'],
                "date": current_date,
                "employee_id": employee_id,
                "total_sales": Decimal(random.randint(200000, 1000000)),
                "total_transactions": random.randint(5, 20),
                "total_items_sold": random.randint(20, 70),
                "commission_earned": Decimal(random.randint(20000, 100000)),
                "performance_score": Decimal(random.uniform(3.5, 5.0)).quantize(Decimal('0.01'))
            })
            id_counters['employee'] += 1
            
        # 4. Buat data customer_analytics (per grup produk per hari)
        for group_id in GROUP_IDS:
            total_revenue = Decimal(random.randint(1000000, 5000000))
            total_transactions = random.randint(50, 150)
            customer_data.append({
                "id": id_counters['customer'],
                "date": current_date,
                "product_group_id": group_id,
                "total_transactions": total_transactions,
                "total_revenue": total_revenue,
                "average_transaction_value": total_revenue / total_transactions,
                "peak_hour": f"{random.randint(11, 20)}:00"
            })
            id_counters['customer'] += 1

    print("Pembuatan data palsu selesai.")
    return sales_data, product_data, employee_data, customer_data

def insert_data(engine, sales_data, product_data, employee_data, customer_data):
    print("Memasukkan data ke database...")
    
    # Siapkan perintah INSERT
    insert_sales_stmt = text("""
        INSERT INTO sales_summary_daily (id, date, cashier_id, total_transactions, total_items_sold, gross_sales, total_discounts, net_sales, total_tax, total_cost, gross_profit)
        VALUES (:id, :date, :cashier_id, :total_transactions, :total_items_sold, :gross_sales, :total_discounts, :net_sales, :total_tax, :total_cost, :gross_profit)
    """)
    
    insert_product_stmt = text("""
        INSERT INTO product_sales_summary (id, date, product_id, product_group_id, quantity_sold, gross_sales, total_discounts, net_sales, total_cost, gross_profit)
        VALUES (:id, :date, :product_id, :product_group_id, :quantity_sold, :gross_sales, :total_discounts, :net_sales, :total_cost, :gross_profit)
    """)
    
    insert_employee_stmt = text("""
        INSERT INTO employee_performance (id, date, employee_id, total_sales, total_transactions, total_items_sold, commission_earned, performance_score)
        VALUES (:id, :date, :employee_id, :total_sales, :total_transactions, :total_items_sold, :commission_earned, :performance_score)
    """)
    
    insert_customer_stmt = text("""
        INSERT INTO customer_analytics (id, date, product_group_id, total_transactions, total_revenue, average_transaction_value, peak_hour)
        VALUES (:id, :date, :product_group_id, :total_transactions, :total_revenue, :average_transaction_value, :peak_hour)
    """)

    # Gunakan transaksi agar jika satu gagal, semua di-rollback
    try:
        with engine.begin() as conn: # .begin() otomatis memulai transaksi
            print("Membersihkan data lama (TRUNCATE)...")
            # CASCADE diperlukan jika ada foreign key
            conn.execute(text("TRUNCATE TABLE sales_summary_daily, product_sales_summary, employee_performance, customer_analytics CASCADE"))
            
            print(f"Memasukkan {len(sales_data)} baris ke sales_summary_daily...")
            conn.execute(insert_sales_stmt, sales_data)
            
            print(f"Memasukkan {len(product_data)} baris ke product_sales_summary...")
            conn.execute(insert_product_stmt, product_data)
            
            print(f"Memasukkan {len(employee_data)} baris ke employee_performance...")
            conn.execute(insert_employee_stmt, employee_data)
            
            print(f"Memasukkan {len(customer_data)} baris ke customer_analytics...")
            conn.execute(insert_customer_stmt, customer_data)
            
        print("Data palsu berhasil dimasukkan!")
    except Exception as e:
        print(f"\nTerjadi error saat memasukkan data: {e}")
        print("Transaksi dibatalkan (rollback).")

def main():
    print("Memulai skrip seeder...")
    
    # 1. Muat .env
    load_dotenv()
    dsn = os.getenv("ANALYTICS_DSN")
    if not dsn:
        print("Error: ANALYTICS_DSN tidak ditemukan di .env")
        return
        
    try:
        url, params = convert_dsn_to_url(dsn)
        print(f"Menghubungkan ke database di {params.get('host', '...')}")
        engine = create_engine(url)
    except Exception as e:
        print(f"Error menghubungkan ke database: {e}")
        return

    # 2. Buat data
    sales_data, product_data, employee_data, customer_data = generate_dummy_data()
    
    # 3. Masukkan data
    insert_data(engine, sales_data, product_data, employee_data, customer_data)
    
    print("Skrip seeder selesai.")

if __name__ == "__main__":
    main()

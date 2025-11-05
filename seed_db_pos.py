import repositories.db_connection # Penting: Baris ini memuat .env
from repositories.db_connection import get_pos_db_session
from sqlalchemy.orm import Session
from sqlalchemy import text
from faker import Faker
import random
import datetime
from decimal import Decimal

# --- KONFIGURASI ---
JUMLAH_TRANSAKSI = 200  # Buat 200 transaksi
JUMLAH_PRODUK = 20     # Buat 20 produk
JUMLAH_KASIR = 3        # Asumsi ada 3 kasir (ID 1, 2, 3)
RENTANG_HARI = 30       # Buat data untuk 30 hari terakhir
# ---------------------

fake = Faker('id_ID') # Menggunakan data Indonesia

def clean_tables(db: Session):
    """Membersihkan tabel POS agar bisa diisi ulang."""
    print("Membersihkan tabel POS ...")
    try:
        # Hapus dalam urutan terbalik (anak dulu) untuk foreign key
        db.execute(text("TRUNCATE order_items RESTART IDENTITY CASCADE"))
        db.execute(text("TRUNCATE order_documents RESTART IDENTITY CASCADE"))
        db.execute(text("TRUNCATE products RESTART IDENTITY CASCADE"))
        db.execute(text("TRUNCATE product_groups RESTART IDENTITY CASCADE"))
        db.commit()
        print("Tabel POS berhasil dibersihkan.")
    except Exception as e:
        db.rollback()
        print(f"Error saat membersihkan tabel: {e}")

def seed_product_groups(db: Session) -> list:
    """Membuat product_groups palsu."""
    print("Membuat data product_groups...")
    grup = ['Makanan Utama', 'Minuman Dingin', 'Minuman Panas', 'Cemilan', 'Dessert']
    group_ids = []
    for nama in grup:
        res = db.execute(
            text("INSERT INTO product_groups (product_group_name) VALUES (:name) RETURNING id"),
            {"name": nama}
        )
        group_ids.append(res.scalar_one())
    db.commit()
    print(f"Dibuat {len(group_ids)} product groups.")
    return group_ids

def seed_products(db: Session, group_ids: list) -> dict:
    """Membuat products palsu dan menyimpan harganya."""
    print(f"Membuat {JUMLAH_PRODUK} data products...")
    product_data = {} # Simpan harga di sini: {id: {'price': ..., 'cost': ...}}
    
    for i in range(JUMLAH_PRODUK):
        nama_produk = fake.food_name()
        cost_price = Decimal(random.randrange(5000, 50000, 1000))
        product_price = cost_price * Decimal(random.uniform(1.3, 2.0)) # Margin 30-100%
        
        res = db.execute(
            text("""
                INSERT INTO products 
                (product_code, product_name, product_price, cost_price, product_group_id)
                VALUES (:code, :name, :price, :cost, :group_id)
                RETURNING id
            """),
            {
                "code": fake.unique.ean(length=8),
                "name": nama_produk,
                "price": product_price.quantize(Decimal('0.01')),
                "cost": cost_price.quantize(Decimal('0.01')),
                "group_id": random.choice(group_ids)
            }
        )
        product_id = res.scalar_one()
        product_data[product_id] = {
            "price": product_price,
            "cost": cost_price
        }
    db.commit()
    print(f"Dibuat {len(product_data)} products.")
    return product_data

def seed_orders_and_items(db: Session, product_data: dict):
    """Membuat order_documents dan order_items palsu."""
    print(f"Membuat {JUMLAH_TRANSAKSI} data order_documents dan item-itemnya...")
    product_ids = list(product_data.keys())
    
    for i in range(JUMLAH_TRANSAKSI):
        # 1. Buat Dokumen (Transaksi)
        cashier_id = random.randint(1, JUMLAH_KASIR)
        orders_date = fake.date_time_between(
            start_date=f'-{RENTANG_HARI}d', 
            end_date='now'
        )
        
        doc_res = db.execute(
            text("""
                INSERT INTO order_documents
                (document_number, cashier_id, orders_date, document_type, paid_status)
                VALUES (:doc_num, :cashier_id, :date, 'sale', 'paid')
                RETURNING id
            """),
            {
                "doc_num": f"ORD-{fake.unique.bban()}",
                "cashier_id": cashier_id,
                "date": orders_date
            }
        )
        doc_id = doc_res.scalar_one()
        
        # 2. Buat Item-item untuk Dokumen ini
        doc_subtotal = Decimal(0)
        doc_total_discount = Decimal(0)
        
        jumlah_item_di_order = random.randint(1, 5)
        
        for _ in range(jumlah_item_di_order):
            prod_id = random.choice(product_ids)
            quantity = random.randint(1, 3)
            unit_price = product_data[prod_id]['price']
            
            # Buat diskon palsu (30% kemungkinan dapat diskon)
            discount_amount = Decimal(0)
            if random.random() < 0.3:
                discount_amount = unit_price * Decimal(random.uniform(0.1, 0.25)) # Diskon 10-25%
                
            price_before_discount = unit_price * quantity
            line_total = (unit_price - discount_amount) * quantity
            
            db.execute(
                text("""
                    INSERT INTO order_items
                    (document_id, product_id, quantity, unit_price, 
                     price_before_discount, discount_amount, line_total)
                    VALUES (:doc_id, :prod_id, :qty, :unit_price, :price_before, :discount, :line_total)
                """),
                {
                    "doc_id": doc_id,
                    "prod_id": prod_id,
                    "qty": quantity,
                    "unit_price": unit_price.quantize(Decimal('0.01')),
                    "price_before": price_before_discount.quantize(Decimal('0.01')),
                    "discount": discount_amount.quantize(Decimal('0.01')),
                    "line_total": line_total.quantize(Decimal('0.01'))
                }
            )
            
            doc_subtotal += line_total
            doc_total_discount += (discount_amount * quantity)
            
        # 3. Hitung total akhir dokumen
        tax_amount = doc_subtotal * Decimal(0.11) # PPN 11%
        total_amount = doc_subtotal + tax_amount
        
        # 4. Update Dokumen dengan total yang benar
        db.execute(
            text("""
                UPDATE order_documents
                SET subtotal = :sub, tax_amount = :tax, 
                    discount_amount = :discount, total_amount = :total
                WHERE id = :doc_id
            """),
            {
                "sub": doc_subtotal.quantize(Decimal('0.01')),
                "tax": tax_amount.quantize(Decimal('0.01')),
                "discount": doc_total_discount.quantize(Decimal('0.01')),
                "total": total_amount.quantize(Decimal('0.01')),
                "doc_id": doc_id
            }
        )
        
        if (i + 1) % 50 == 0:
            print(f"  ... {i+1} / {JUMLAH_TRANSAKSI} dokumen dibuat.")

    db.commit()
    print("Pembuatan order dan item selesai.")

def main():
    db = None
    try:
        db = get_pos_db_session()
        print("Terhubung ke database POS ...")
        
        # Hati-hati: Ini akan menghapus data di tabel POS Anda!
        clean_tables(db) 
        
        group_ids = seed_product_groups(db)
        product_data = seed_products(db, group_ids)
        seed_orders_and_items(db, product_data)
        
        print("\n--- Seeding Database POS Selesai! ---")
        
    except Exception as e:
        if db:
            db.rollback()
        print(f"\nTerjadi error: {e}")
        import traceback
        traceback.print_exc()
        
    finally:
        if db:
            db.close()
            print("Koneksi database POS ditutup.")

if __name__ == "__main__":
    main()

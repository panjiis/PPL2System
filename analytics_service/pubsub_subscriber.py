# pubsub_subscriber.py
import asyncio
import json
import string
import nats
from nats.errors import NoServersError
from decimal import Decimal
from dateutil import parser # Untuk parsing ISO 8601 time
from sqlalchemy import text, func
from sqlalchemy.orm import Session
from sqlalchemy.exc import IntegrityError
from sqlalchemy.dialects.postgresql import insert as pg_insert

from repositories.db_connection import get_analytics_db_session
from repositories.models import RawSalesEvent, RawOrderDocument, RawOrderItem, RawProduct, RawFinalizedCommission

from core_logic.cache_manager import redis_client
from core_logic.employee_logic import generate_employee_performance_logic

# Anda bilang akan mengisinya sendiri
NATS_URL = "nats://10.147.17.76:4222" 

LOW_STOCK_HASH_KEY = "dashboard:low_stock_hash"
HARDCODED_LOW_STOCK_LEVEL = 10

PENDING_COMMISSIONS_KEY = "dashboard:pending_commissions"

async def commission_event_handler(msg):
    """Callback untuk memproses SEMUA event komisi."""

    if not redis_client:
        print("Koneksi Redis tidak tersedia, melewati event komisi.")
        return
        
    try:
        data = json.loads(msg.data.decode())
        event_type = data.get("event_type")

        # === LOGIKA 1: INCR (+1) UNTUK "PENDING" ===
        if event_type == "commission.calculated":
            try:
                redis_client.incr(PENDING_COMMISSIONS_KEY)
                print(f"Event commission.calculated diterima, PENDING_COMMISSIONS_KEY di-INCR.")
            except Exception as e:
                print(f"Gagal INCR PENDING_COMMISSIONS_KEY: {e}")
            return # Selesai

        # === LOGIKA 2: DECR (-1) UNTUK "STATUS UPDATE" ===
        elif event_type == "commission.status.updated":
            try:
                redis_client.decr(PENDING_COMMISSIONS_KEY)
                print(f"Event commission.status.updated diterima, PENDING_COMMISSIONS_KEY di-DECR.")
            except Exception as e:
                print(f"Gagal DECR PENDING_COMMISSIONS_KEY: {e}")
            # JANGAN return, biarkan event 'finalized' (jika ada) diproses juga
        
        # === LOGIKA 3: SIMPAN DATA ETL (YANG SUDAH ADA) ===
        if event_type == "commission.finalized":
            calc_id = data.get("calculation_id")
            emp_id = data.get("employee_id")
            p_start = data.get("period_start")
            p_end = data.get("period_end")
            total_sales = data.get("total_sales")
            total_comm = data.get("total_commission")

            if not all([calc_id, emp_id, p_start, p_end, total_sales, total_comm]):
                print("Event commission.finalized tidak lengkap, skipping.")
                return

            stmt = pg_insert(RawFinalizedCommission).values(
                calculation_id=calc_id,
                employee_id=emp_id,
                period_start=p_start,
                period_end=p_end,
                commission_earned=Decimal(total_comm),
                total_sales=Decimal(total_sales)
            ).on_conflict_do_update(
                index_elements=['calculation_id'],
                set_={
                    "commission_earned": Decimal(total_comm),
                    "total_sales": Decimal(total_sales),
                    "processed_at": func.now()
                }
            )
            
            db = None
            try:
                db = get_analytics_db_session()
                db.execute(stmt)
                db.commit()
                print(f"✅ [1/2] Raw Commission Data saved (CalcID: {calc_id})")

                print(f"▶️ [2/2] Generating Employee Performance for CalcID: {calc_id}...")
                
                # Panggil logika bisnis langsung (tanpa lewat gRPC network)
                result = generate_employee_performance_logic(
                    analytics_db=db, # Pass session database
                    calculation_id=calc_id
                )
                
                print(f"✅ [2/2] Performance Generated Successfully. Employee ID: {result.get('employee_id')}")
            except Exception as e:
                if db: db.rollback()
                print(f"DB Error saat memproses event komisi: {e}")
            finally:
                if db: db.close()
            return # Selesai
            
    except Exception as e:
        print(f"Error di commission_event_handler: {e}")

async def inventory_event_handler(msg):
    """Callback HANYA untuk memproses event stok."""
    if not redis_client:
        print("Redis Connection is unavailable, skipping stock event.")
        return
        
    try:
        data = json.loads(msg.data.decode())
        event_type = data.get("event_type")

        if event_type == "inventory.stock.updated":
            product_name_raw = data.get("product_name")
            new_quantity = data.get("new_quantity")

            if not all([product_name_raw, new_quantity is not None]):
                print("Event inventory.stock.updated incomplete, skipping.")
                return
            
            product_name = "".join(filter(lambda c: c in string.printable, product_name_raw)).strip()

            # --- PILIH LOGIKA ANDA (A atau B) ---

            # == OPSI A (Logika Hardcoded <= 10) ==
            # (Jika Anda memilih ini, hapus Opsi B)
            is_low_stock = (new_quantity <= HARDCODED_LOW_STOCK_LEVEL)
            
            # == OPSI B (Logika ReorderLevel) ==
            # (Jika Anda memilih ini, hapus Opsi A)
            # reorder_level = data.get("reorder_level")
            # if reorder_level is None:
            #     print("Event stok tidak memiliki 'reorder_level', skipping.")
            #     return
            # is_low_stock = (new_quantity <= reorder_level)
            
            # --- Logika Redis ---
            if is_low_stock:
                # Simpan (key, field, value) -> (hash_key, "Ayam Rica", 4)
                redis_client.hset(LOW_STOCK_HASH_KEY, product_name, new_quantity)
                print(f"Low stock detected: {product_name} (Left: {new_quantity})")
            else:
                # Hapus produk dari HASH (jika stoknya tidak lagi rendah)
                redis_client.hdel(LOW_STOCK_HASH_KEY, product_name)
                
    except Exception as e:
        print(f"Error di inventory_event_handler: {e}")

# async def commission_event_handler(msg):
#     """Callback HANYA untuk memproses event komisi."""
#     try:
#         data = json.loads(msg.data.decode())
#         event_type = data.get("event_type")

#         if event_type == "commission.finalized":
#             # Ambil data (gunakan snake_case sesuai tag JSON di Go)
#             calc_id = data.get("calculation_id")
#             emp_id = data.get("employee_id")
#             p_start = data.get("period_start")
#             p_end = data.get("period_end")
#             total_sales = data.get("total_sales")
#             total_comm = data.get("total_commission")

#             if not all([calc_id, emp_id, p_start, p_end, total_sales, total_comm]):
#                 print("Event commission.finalized incomplete, skipping.")
#                 return

#             # Buat pernyataan Upsert
#             stmt = pg_insert(RawFinalizedCommission).values(
#                 calculation_id=calc_id,
#                 employee_id=emp_id,
#                 period_start=p_start,
#                 period_end=p_end,
#                 commission_earned=Decimal(total_comm),
#                 total_sales=Decimal(total_sales)
#             ).on_conflict_do_update(
#                 index_elements=['calculation_id'], # Konflik pada calculation_id
#                 set_={
#                     "commission_earned": Decimal(total_comm),
#                     "total_sales": Decimal(total_sales),
#                     "processed_at": func.now()
#                 }
#             )
            
#             db = None
#             try:
#                 db = get_analytics_db_session()
#                 db.execute(stmt)
#                 db.commit()
#                 print(f"Success on processing final commission (CalcID: {calc_id})")
#             except Exception as e:
#                 if db: db.rollback()
#                 print(f"DB Error while processing commission event: {e}")
#             finally:
#                 if db: db.close()
            
#             return
            
#     except Exception as e:
#         print(f"Error in commission_event_handler: {e}")

async def message_handler(msg):
    """Callback function untuk memproses pesan NATS yang masuk."""
    try:
        data = json.loads(msg.data.decode())
        event_type = data.get("event_type")

        # --- BLOK 1: TANGANI EVENT PRODUK ---
        if event_type == "pos.product.created" or event_type == "pos.product.updated":
            product_data = data.get("product_data")
            if not product_data:
                print("Event product doesn't have payload 'product_data'")
                return
            
            prod_code = product_data.get("product_code")
            group_id = product_data.get("product_group_id")
            prod_name = product_data.get("product_name")
            cost_price = Decimal(product_data.get("cost_price", 0))

            if not prod_code:
                print("Payload event product doesn't have 'product_code'")
                return

            stmt = pg_insert(RawProduct).values(
                product_code=prod_code,
                product_group_id=group_id,
                product_name=prod_name,
                cost_price=cost_price
            ).on_conflict_do_update(
                index_elements=['product_code'],
                set_={
                    "product_group_id": group_id,
                    "product_name": prod_name,
                    "cost_price": cost_price,
                    "processed_at": func.now()
                }
            )
            
            # (BARIS 'order_timestamp' DAN 'total_amount' YANG SALAH TELAH DIHAPUS DARI SINI)
            
            db = None
            try:
                db = get_analytics_db_session()
                db.execute(stmt)
                db.commit()
                print(f"Success on processing product event: {prod_code}")
            except Exception as e:
                if db: db.rollback()
                print(f"DB Error while processing product event: {e}")
            finally:
                if db: db.close()
            return # Selesai memproses event produk

        # --- BLOK 2: TANGANI EVENT ORDER ---
        if event_type == "order.created":
            order_data = data.get("order_data")
            if not order_data:
                print("Message 'order.created' doesn't have 'OrderData' payload, skipping.")
                return
            
            items_data = order_data.get("OrderItems", [])
            if not items_data:
                print("OrderData tidak memiliki 'OrderItems', skipping.")
                return
            
            doc_number = order_data.get("DocumentNumber")
            timestamp_str = order_data.get("OrdersDate")
            total_amount_str = order_data.get("TotalAmount")
            cashier_id = order_data.get("CashierId")
            tax_amount_str = order_data.get("TaxAmount")
            order_id = order_data.get("ID")

            if not all([doc_number, timestamp_str, total_amount_str, cashier_id, order_id, tax_amount_str]):
                print("Data event on 'order_data' incomplete, skipping.")
                return

            # Konversi data
            order_timestamp = parser.isoparse(timestamp_str)
            total_amount = Decimal(total_amount_str)
            total_amount_float = float(total_amount) # <-- Definisikan di sini
            tax_amount = Decimal(tax_amount_str)

            if redis_client:
                try:
                    dashboard_main_key = order_timestamp.strftime("dashboard:main:%Y-%m-%d")
                    today_key = order_timestamp.strftime("metrics:%Y-%m-%d") # Key untuk grafik
                    current_hour = str(order_timestamp.hour)
                    
                    pipe = redis_client.pipeline()
                    
                    pipe.hincrbyfloat(today_key, f"hour_revenue:{current_hour}", total_amount_float)
                    pipe.hincrby(today_key, f"hour_count:{current_hour}", 1)
                    
                    pipe.hincrbyfloat(today_key, "total_revenue", total_amount_float) 
                    pipe.expire(today_key, 86400 * 3)

                    pipe.delete(dashboard_main_key)

                    peak_hours_pattern = "reports:weekly-peak-hours:*"
                    for key in redis_client.scan_iter(match=peak_hours_pattern):
                        pipe.delete(key)
                    
                    pipe.execute()
                    print(f"Update Metrics & Delete Cache Dashboard: {doc_number}")

                except Exception as e:
                    print(f"Redis Error: {e}")

            # --- LOGIKA REDIS (SEKARANG DI TEMPAT YANG BENAR) ---
            if not redis_client:
                print("Koneksi Redis tidak tersedia, melewati pembaruan metrik real-time.")
            else:
                try:
                    today_key = order_timestamp.strftime("metrics:%Y-%m-%d")
                    current_hour = str(order_timestamp.hour)
                    pipe = redis_client.pipeline()
                    pipe.hincrbyfloat(today_key, "total_revenue", total_amount_float)
                    pipe.hincrby(today_key, "total_transactions", 1)
                    pipe.hincrbyfloat(today_key, f"hour_revenue:{current_hour}", total_amount_float)
                    pipe.hincrby(today_key, f"hour_count:{current_hour}", 1)
                    if total_amount > 1000000:
                        tx_info = json.dumps({"doc": doc_number, "amount": total_amount_str})
                        pipe.lpush("metrics:realtime:large_tx", tx_info)
                        pipe.ltrim("metrics:realtime:large_tx", 0, 9)
                    pipe.set("metrics:realtime:last_updated", order_timestamp.isoformat())
                    pipe.expire(today_key, 86400 * 3)
                    pipe.execute()
                except Exception as e:
                    print(f"REDIS Error saat memproses metrik real-time: {e}")
            # --- AKHIR LOGIKA REDIS ---

            # Simpan ke Database Analytics
            db: Session = None
            try:
                db = get_analytics_db_session()
                
                # ... (Blok 'stmt_sales_event' Anda) ...
                stmt_sales_event = pg_insert(RawSalesEvent).values(
                    order_document_number=doc_number,
                    order_timestamp=order_timestamp,
                    total_amount=total_amount,
                    cashier_id=cashier_id
                ).on_conflict_do_nothing(index_elements=['order_document_number'])
                db.execute(stmt_sales_event)
                
                # ... (Blok 'stmt_doc' Anda) ...
                stmt_doc = pg_insert(RawOrderDocument).values(
                    id=order_id,
                    document_number=doc_number,
                    cashier_id=cashier_id,
                    order_timestamp=order_timestamp,
                    tax_amount=tax_amount
                ).on_conflict_do_nothing(index_elements=['document_number'])
                db.execute(stmt_doc)

                # ... (Blok 'new_items' Anda) ...
                new_items = []
                for item in items_data:
                    cost_price_str = item.get("cost_price") 
                    serving_emp_id = item.get("serving_employee_id")
                    
                    if cost_price_str is None:
                        product_code = item.get("product_code") 
                        print(f"Item {product_code} on order {doc_number} doesn't have 'cost_price' in payload. Skipping.")
                        raise ValueError("Item payload incomplete")

                    new_items.append({
                        "document_number": doc_number,
                        "product_code": item.get("product_code"),
                        "product_name": item.get("product_name"),
                        "serving_employee_id": serving_emp_id,
                        "quantity": item.get("quantity"),
                        "price_before_discount": Decimal(item.get("price_before_discount")),
                        "discount_amount": Decimal(item.get("discount_amount")),
                        "line_total": Decimal(item.get("line_total")),
                        "cost_price": Decimal(cost_price_str)
                    })

                print(f"new item : \n {new_items}")
                
                if new_items:
                    db.execute(text("DELETE FROM raw_order_items WHERE document_number = :doc_num"), {"doc_num": doc_number})
                    db.bulk_insert_mappings(RawOrderItem, new_items)

                db.commit()
                print(f"Success on processing order event: {doc_number}") # <-- Print konsisten

            except Exception as e:
                if db: db.rollback()
                print(f"DB Error while processing order event: {e}")
            finally:
                if db: db.close()
            return # Selesai memproses event order

    except Exception as e:
        print(f"Error di message_handler: {e}")

async def user_event_handler(msg):
    """
    Callback untuk menangani event employee dari User Service.
    Topic: employee.created, employee.updated
    """
    if not redis_client: 
        return

    try:
        # 1. Parsing Data
        data = json.loads(msg.data.decode())
        subject = msg.subject # Contoh: "employee.created"

        # Cek apakah ini event yang kita butuhkan
        if subject in ["employee.created", "employee.updated"]:
            
            # Mapping Field dari Go (employee.go) -> ke DB Analytics (raw_employees)
            emp_id = data.get("id")
            name = data.get("employee_name") # Di Go namanya 'employee_name'
            role = data.get("position")      # Di Go namanya 'position', di Analytics 'role'
            
            if not emp_id or not name:
                print(f"⚠️ Event {subject} incomplete data: {data}")
                return

            # 2. UPSERT ke tabel raw_employees
            # Logika: Jika ID sudah ada, update Nama & Role. Jika belum, Insert.
            stmt = text("""
                INSERT INTO raw_employees (employee_id, name, role, updated_at)
                VALUES (:id, :name, :role, NOW())
                ON CONFLICT (employee_id) 
                DO UPDATE SET 
                    name = EXCLUDED.name, 
                    role = EXCLUDED.role, 
                    updated_at = NOW()
            """)
            
            db = None
            try:
                db = get_analytics_db_session()
                db.execute(stmt, {"id": emp_id, "name": name, "role": role})
                db.commit()
                print(f"✅ Employee synced via NATS: {name} (ID: {emp_id})")
            except Exception as e:
                print(f"❌ DB Error syncing employee: {e}")
            finally:
                if db: db.close()

    except Exception as e:
        print(f"❌ Error user_event_handler: {e}")

async def run_subscriber():
    """Menghubungkan ke NATS dan memulai subscriber."""
    print(f"Connecting ke NATS in {NATS_URL}...")
    
    # --- TENTUKAN SEMUA TOPIK ---
    pos_topic = "pos.>"
    commission_topic = "commission.>"  
    inventory_topic = "inventory.stock.updated" 
    employee_topic = "employee.>"
    
    try:
        nc = await nats.connect(NATS_URL)
        print("Connected to NATS.")
        
        # --- SUBSCRIBE KE TOPIK 1 ---
        await nc.subscribe(pos_topic, cb=message_handler)
        print(f"Subscribed to topic: '{pos_topic}'")

        # --- SUBSCRIBE KE TOPIK 2 ---
        await nc.subscribe(commission_topic, cb=commission_event_handler)
        print(f"Subscribed to topic: '{commission_topic}'")

        # --- SUBSCRIBE KE TOPIK 3 ---
        await nc.subscribe(inventory_topic, cb=inventory_event_handler)
        print(f"Subscribed to topic: '{inventory_topic}'")
        
        # --- SUBSCRIBE KE TOPIK 4 ---
        await nc.subscribe(employee_topic, cb=user_event_handler)
        print(f"Subscribed to topic: '{employee_topic}'")

        # Biarkan subscriber berjalan selamanya
        await asyncio.Event().wait()
    except NoServersError:
        print(f"Unnable to connect to NATS server in {NATS_URL}. Ensure NATS is running.")
    except Exception as e:
        print(f"NATS connection error: {e}")
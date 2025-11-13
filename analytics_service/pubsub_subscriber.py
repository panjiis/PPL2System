# pubsub_subscriber.py
import asyncio
import json
import nats
from nats.errors import NoServersError
from decimal import Decimal
from dateutil import parser # Untuk parsing ISO 8601 time
from sqlalchemy import text
from sqlalchemy.orm import Session
from sqlalchemy.exc import IntegrityError
from sqlalchemy.dialects.postgresql import insert as pg_insert

from repositories.db_connection import get_analytics_db_session
from repositories.models import RawSalesEvent, RawOrderDocument, RawOrderItem # Import model dari Langkah 3
from core_logic.cache_manager import delete_cache

# Anda bilang akan mengisinya sendiri
NATS_URL = "nats://10.147.17.76:4222" 
SUBSCRIBE_TOPIC = "pos.order.events.all"

async def message_handler(msg):
    """Callback function untuk memproses pesan NATS yang masuk."""
    try:
        data = json.loads(msg.data.decode())

        # 1. Filter event yang relevan (sesuai konfirmasi Anda)
        event_type = data.get("event_type")
        if event_type != "order.created":
            # print(f"Skipping event type: {event_type}")
            return

        # 2. Ekstrak payload
        order_data = data.get("order_data")
        if not order_data:
            print("Message 'order.created' doesn't have 'OrderData' payload, skipping.")
            return
        
        items_data = order_data.get("OrderItems", [])
        if not items_data:
            print("OrderData tidak memiliki 'OrderItems', skipping.")
            return
        
        # 3. Ambil data spesifik (sesuai konfirmasi Anda)
        doc_number = order_data.get("DocumentNumber")
        timestamp_str = order_data.get("OrdersDate") # Ini adalah timestamp
        total_amount_str = order_data.get("TotalAmount")
        cashier_id = order_data.get("CashierId")
        tax_amount_str = order_data.get("TaxAmount") # Diperlukan untuk ETL
        order_id = order_data.get("ID") # ID asli dari POS

        if not all([doc_number, timestamp_str, total_amount_str, cashier_id, order_id, tax_amount_str]):
            print("Data event 'order_data' tidak lengkap, skipping.")
            return

        # 4. Konversi data
        order_timestamp = parser.isoparse(timestamp_str)
        total_amount = Decimal(total_amount_str)
        tax_amount = Decimal(tax_amount_str)

        # 5. Simpan ke Database Analytics
        db: Session = None
        try:
            db = get_analytics_db_session()

            # --- Bagian 1: Simpan ke raw_sales_events (Untuk GetPeakHours) ---
            # Kita tetap lakukan ini agar GetPeakHours berfungsi
            stmt_sales_event = pg_insert(RawSalesEvent).values(
                order_document_number=doc_number,
                order_timestamp=order_timestamp,
                total_amount=total_amount,
                cashier_id=cashier_id
            ).on_conflict_do_nothing(index_elements=['order_document_number'])
            db.execute(stmt_sales_event)
            
            # --- Bagian 2: Simpan ke raw_order_documents (Untuk ETL) ---
            stmt_doc = pg_insert(RawOrderDocument).values(
                id=order_id, # Simpan ID asli
                document_number=doc_number,
                cashier_id=cashier_id,
                order_timestamp=order_timestamp,
                tax_amount=tax_amount
            ).on_conflict_do_nothing(index_elements=['document_number'])
            db.execute(stmt_doc)

            # --- Bagian 3: Simpan ke raw_order_items (Untuk ETL) ---
            new_items = []
            for item in items_data:
                # INI ASUMSI PERUBAHAN DI GO (Langkah 2)
                cost_price_str = item.get("cost_price") 
                
                # PERIKSA DATA KUNCI
                if cost_price_str is None:
                    product_code = item.get("product_code") 
                    print(f"Item {product_code} di order {doc_number} tidak memiliki 'cost_price' di payload. Skipping.")
                    raise ValueError("Payload item tidak lengkap")

                new_items.append({
                    "document_number": doc_number,
                    "product_code": item.get("product_code"),
                    "quantity": item.get("quantity"),
                    "price_before_discount": Decimal(item.get("price_before_discount")),
                    "discount_amount": Decimal(item.get("discount_amount")),
                    "line_total": Decimal(item.get("line_total")),
                    "cost_price": Decimal(cost_price_str) # Data Kunci!
                })

            if new_items:
                # Hapus item lama jika ada (untuk idempotency)
                db.execute(text("DELETE FROM raw_order_items WHERE document_number = :doc_num"), {"doc_num": doc_number})
                # Masukkan item baru
                db.bulk_insert_mappings(RawOrderItem, new_items)

            db.commit()
            print(f"Berhasil memproses event order LENGKAP: {doc_number}")
            delete_cache("reports:*")

        except Exception as e:
            if db: db.rollback()
            print(f"DB Error saat memproses message: {e}")
        finally:
            if db: db.close()

    except Exception as e:
        print(f"Error di message_handler: {e}")

async def run_subscriber():
    """Menghubungkan ke NATS dan memulai subscriber."""
    print(f"Connecting ke NATS in {NATS_URL}...")
    try:
        nc = await nats.connect(NATS_URL)
        print(f"Connected. Subscribe to topic: '{SUBSCRIBE_TOPIC}'")
        
        await nc.subscribe(SUBSCRIBE_TOPIC, cb=message_handler)
        
        # Biarkan subscriber berjalan selamanya
        await asyncio.Event().wait() 
        
    except NoServersError:
        print(f"Unnable to connect to NATS server in {NATS_URL}. Ensure NATS is running.")
    except Exception as e:
        print(f"NATS connection error: {e}")
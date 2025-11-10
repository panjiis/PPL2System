# pubsub_subscriber.py
import asyncio
import json
import nats
from nats.errors import NoServersError
from decimal import Decimal
from dateutil import parser # Untuk parsing ISO 8601 time
from sqlalchemy.orm import Session
from sqlalchemy.exc import IntegrityError

from repositories.db_connection import get_analytics_db_session
from repositories.models import RawSalesEvent # Import model dari Langkah 3

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

        # 3. Ambil data spesifik (sesuai konfirmasi Anda)
        doc_number = order_data.get("DocumentNumber")
        timestamp_str = order_data.get("OrdersDate") # Ini adalah timestamp
        total_amount_str = order_data.get("TotalAmount")
        cashier_id = order_data.get("CashierId")

        if not all([doc_number, timestamp_str, total_amount_str, cashier_id]):
            print("Incomplete event data, skipping.")
            return

        # 4. Konversi data
        order_timestamp = parser.isoparse(timestamp_str)
        total_amount = Decimal(total_amount_str)

        # 5. Simpan ke Database Analytics
        db: Session = None
        try:
            db = get_analytics_db_session()

            new_event = RawSalesEvent(
                order_document_number=doc_number,
                order_timestamp=order_timestamp,
                total_amount=total_amount,
                cashier_id=cashier_id
            )
            
            db.add(new_event)
            db.commit()
            print(f"Event order saved successfully: {doc_number}")

        except IntegrityError:
            # Ini terjadi jika document_number sudah ada (UNIQUE constraint)
            # Ini adalah hal yang baik (idempotency), kita abaikan saja.
            if db: db.rollback()
            print(f"duplicate event for order {doc_number}, skipping.")
        except Exception as e:
            if db: db.rollback()
            print(f"DB Error while processing message: {e}")
        finally:
            if db: db.close()

    except json.JSONDecodeError:
        print("Failed to decode JSON message")
    except Exception as e:
        print(f"Error in message_handler: {e}")

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
import os
from dotenv import load_dotenv
from sqlalchemy import create_engine, text
from sqlalchemy.exc import ProgrammingError

try:
    from repositories.db_connection import convert_dsn_to_url
except ImportError:
    print("Error: Tidak dapat mengimpor 'convert_dsn_to_url'.")
    print("Pastikan file 'repositories/db_connection.py' ada dan benar.")
    exit(1)

SQL_FILE_PATH = 'SYNTRA_analytics.sql'

def run_migration():
    print("Memulai migrasi...")
    
    load_dotenv()
    
    dsn = os.getenv("ANALYTICS_DSN")
    if not dsn:
        print("Error: ANALYTICS_DSN tidak ditemukan di .env")
        return
        
    try:
        url, params = convert_dsn_to_url(dsn)
        print(f"Menghubungkan ke database di {params.get('host', '...')}")
    except Exception as e:
        print(f"Error mem-parsing DSN: {e}")
        return

    try:
        with open(SQL_FILE_PATH, 'r', encoding='utf-8') as f:
            sql_commands = f.read()
            print(f"File '{SQL_FILE_PATH}' berhasil dibaca.")
    except FileNotFoundError:
        print(f"Error: File '{SQL_FILE_PATH}' tidak ditemukan.")
        print("Pastikan file tersebut ada di direktori yang sama dengan migrate.py")
        return
    except Exception as e:
        print(f"Error membaca file SQL: {e}")
        return

    try:
        engine = create_engine(url, isolation_level="AUTOCOMMIT")
        with engine.connect() as conn:
            conn.execute(text(sql_commands))
        print("Migrasi berhasil! Semua tabel telah dibuat.")
        
    except ProgrammingError as pe:
        if "already exists" in str(pe).lower():
            print("\nMigrasi Gagal: Tampaknya tabel/schema sudah ada.")
            print("Detail:", pe)
        else:
            print(f"\nError SQL Programming: {pe}")
    except Exception as e:
        print(f"\nError saat menjalankan migrasi: {e}")

if __name__ == "__main__":
    run_migration()

import time
import sys
import signal
from core_logic.cache_manager import clear_dashboard_caches

# Impor server gRPC Anda
import grpc_server 

def serve():
    """
    Fungsi utama untuk menjalankan server gRPC.
    """
    print("Starting gRPC Analytics Service...")
    
    # Panggil fungsi start_grpc_server dari file grpc_server.py
    # Ini akan berjalan di port 50055
    grpc_server.start_grpc_server()
    print(f"gRPC Server Running on {grpc_server.GRPC_PORT}")

    # Jaga agar thread utama tetap hidup
    # server.start() tidak memblokir, jadi kita perlu cara
    # untuk menjaga aplikasi tetap berjalan.
    try:
        while True:
            time.sleep(86400) # Tidur selama satu hari
    except KeyboardInterrupt:
        # Ini tidak akan tertangkap jika server.start() memblokir,
        # jadi kita juga butuh signal handler.
        pass

def handle_shutdown(sig, frame):
    """Menangani sinyal shutdown (seperti Ctrl+C) dengan bersih."""
    print("\nStop command received. Stopping service...")
    grpc_server.stop_grpc_server()
    try:
        clear_dashboard_caches()
    except Exception as e:
        print(f"Error clearing cache: {e}")
    print("Analytics gRPC service stopped.")
    sys.exit(0)

if __name__ == '__main__':
    # Menambahkan signal handler untuk Ctrl+C
    signal.signal(signal.SIGINT, handle_shutdown)
    signal.signal(signal.SIGTERM, handle_shutdown)
    
    serve()
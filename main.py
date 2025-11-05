# main.py
import uvicorn
import threading
from contextlib import asynccontextmanager

from grpc_server import start_grpc_server, stop_grpc_server
from api_server import app  

HTTP_PORT = 8000

@asynccontextmanager
async def lifespan(app_instance):
    print("FastAPI application starting up...")
    start_grpc_server()

    try:
        yield
    finally:
        print("FastAPI application shutting down...")
        stop_grpc_server()

app.router.lifespan = lifespan

if __name__ == "__main__":
    print(f"Running server on --reload mode")
    print(f"FastAPI HTTP server running on http://0.0.0.0:{HTTP_PORT}")

    uvicorn.run(
        "main:app",
        host="0.0.0.0",
        port=HTTP_PORT,
        reload=True
    )
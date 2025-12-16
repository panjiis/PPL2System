import redis
import os
import json
from dotenv import load_dotenv
from decimal import Decimal
import datetime

load_dotenv()

REDIS_HOST = os.environ.get("REDIS_HOST", "localhost")
REDIS_PORT = int(os.environ.get('REDIS_PORT', 6279))

class CustomJSONEncoder(json.JSONEncoder):
  def default(self, obj):
    if isinstance(obj, Decimal):
      return str(obj)
    if isinstance(obj, (datetime.date, datetime.datetime)):
      return obj.isoformat()
    return super().default(obj)

redis_client = None
try:
  redis_client = redis.Redis(
    host=REDIS_HOST,
    port=REDIS_PORT,
    db=0,
    decode_responses=True,
    socket_timeout=5,
  )
  redis_client.ping()
  print("Successfully connected to Redis!")

except redis.exceptions.ConnectionError as e:
  print(f"Could not connect to Redis: {e}")
  redis_client = None
except Exception as e:
  print(f"An unexpected error occured with Redis: {e}")
  redis_client = None

def set_cache(
  key: str,
  value,
  ttl_seconds: int = 600
):
  if not redis_client:
    return False
  
  try:
    json_value = json.dumps(value, cls=CustomJSONEncoder)
    redis_client.setex(name=key, time=ttl_seconds, value=json_value)
    return True
  except Exception as e:
    print(f"Error setting cache for key '{key}: {e}")
    return False
  
def get_cache(key: str):
  if not redis_client:
    return False
  
  try:
    cached_value = redis_client.get(key)
    if cached_value:
      return json.loads(cached_value)
    return None
  except Exception as e:
    print(f"Error getting cache for key '{key}: {e}")
    return None

def delete_cache(key_pattern: str):
  if not redis_client:
    return False
  
  try:
    for key in redis_client.scan_iter(key_pattern):
      redis_client.delete(key)
      print("Cache deleted for key:", key)
    return True
  except Exception as e:
    print(f"Error deleting cache with pattern '{key_pattern}: {e}")
    return False
  
def clear_all_analytics_caches():
  if not redis_client:
    print("Redis client not connected.")
    return

  print("🧹 Cleaning up ALL analytics caches...")
  
  # Daftar pattern key yang mencakup seluruh sistem analytics
  patterns = [
    "dashboard:*",  # Main Dashboard, Low Stock, Pending Comm
    "metrics:*",    # Realtime Charts
    "reports:*"     # Sales Report, Product Summary, Peak Hours, dll
  ]
  
  count = 0
  try:
    # Menggunakan pipeline untuk menghapus banyak key sekaligus (lebih cepat)
    pipe = redis_client.pipeline()
    found_keys = False

    for pattern in patterns:
      # scan_iter aman untuk production (tidak memblokir Redis)
      for key in redis_client.scan_iter(match=pattern):
        pipe.delete(key)
        count += 1
        found_keys = True
        
        # Eksekusi batch setiap 100 key agar memori python aman
        if count % 100 == 0:
          pipe.execute()
          pipe = redis_client.pipeline()

    if found_keys:
      pipe.execute() # Hapus sisa key
        
    print(f"✅ Cache cleanup complete. Removed {count} keys.")
  except Exception as e:
    print(f"❌ Error during cache cleanup: {e}")

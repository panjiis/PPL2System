from dotenv import load_dotenv
from sqlalchemy import create_engine
from sqlalchemy.orm import sessionmaker, Session
from sqlalchemy.exc import OperationalError
import os

load_dotenv()

def convert_dsn_to_url(dsn: str) -> tuple[str, dict]:
    if not dsn:
        return "", {}
        
    try:
        params = dict(item.split('=') for item in dsn.split(' '))
        
        required_keys = ['host', 'user', 'password', 'dbname', 'port']
        for key in required_keys:
            if key not in params:
                raise ValueError(f"Missing required DSN parameter: {key}")

        url = (
            f"postgresql://{params['user']}:{params['password']}"
            f"@{params['host']}:{params['port']}/{params['dbname']}"
        )
        return url, params
        
    except Exception as e:
        print(f"Error parsing DSN: {e}")
        return "", {}

# --- Connection To Analytics Microsservice ---
ANALYTICS_DSN = os.getenv("ANALYTICS_DSN")
ANALYTICS_DB_URL, analytics_params = convert_dsn_to_url(ANALYTICS_DSN)

if not ANALYTICS_DB_URL:
    print("Error: ANALYTICS_DSN is not valid or not found in .env")
    exit(1)

try:
    analytics_engine = create_engine(ANALYTICS_DB_URL, pool_pre_ping=True)
    AnalyticsSessionLocal = sessionmaker(autocommit=False, autoflush=False, bind=analytics_engine)
    
    with analytics_engine.connect() as conn:
        print(f"Database connection (Analytics) successful to {analytics_params.get('host', 'unknown')}.")
        
except OperationalError as e:
    print(f"Error connecting to Analytics database: {e}")
    exit(1)

def get_analytics_db():
    db = AnalyticsSessionLocal()
    try:
        yield db
    finally:
        db.close()

def get_analytics_db_session() -> Session:
    return AnalyticsSessionLocal()

# --- Connection To POS Microservice ---
POS_DSN = os.getenv("POS_DSN")
POS_DB_URL, pos_params = convert_dsn_to_url(POS_DSN)

if not POS_DB_URL:
    print("Error: POS_DSN is not valid or not found in .env")
    exit(1)

try:
    pos_engine = create_engine(POS_DB_URL, pool_pre_ping=True)
    POSSessionLocal = sessionmaker(autocommit=False, autoflush=False, bind=pos_engine)
    
    with pos_engine.connect() as conn:
        print(f"Database connection (POS) successful to {pos_params.get('host', 'unknown')}.")
        
except OperationalError as e:
    print(f"Error connecting to POS database: {e}")
    exit(1)

def get_pos_db():
    db = POSSessionLocal()
    try:
        yield db
    finally:
        db.close()

def get_pos_db_session() -> Session:
    return POSSessionLocal()

# --- Connection To Inventory Microservice ---
INVENTORY_DSN = os.getenv("INVENTORY_DSN")
INVENTORY_DB_URL, inventory_params = convert_dsn_to_url(INVENTORY_DSN)

if not INVENTORY_DB_URL:
    print("Error: INVENTORY_DSN is not valid or not found in .env")
    exit(1)

try:
    inventory_engine = create_engine(INVENTORY_DB_URL, pool_pre_ping=True)
    InventorySessionLocal = sessionmaker(autocommit=False, autoflush=False, bind=inventory_engine)
    
    with inventory_engine.connect() as conn:
        print(f"Database connection (Inventory) successful to {inventory_params.get('host', 'unknown')}.")

except OperationalError as e:
    print(f"Error connecting to Inventory database: {e}")
    exit(1)

def get_inventory_db():
    db = InventorySessionLocal()
    try:
        yield db
    finally:
        db.close()

def get_inventory_db_session() -> Session:
    return InventorySessionLocal()

# --- Connection To Commissions Microservice ---
COMMISSIONS_DSN = os.getenv("COMMISSIONS_DSN")
COMMISSIONS_DB_URL, commissions_params = convert_dsn_to_url(COMMISSIONS_DSN)

if not COMMISSIONS_DB_URL:
    print("Error: COMMISSIONS_DSN is not valid or not found in .env")
    exit(1)

try:
    commissions_engine = create_engine(COMMISSIONS_DB_URL, pool_pre_ping=True)
    CommissionsSessionLocal = sessionmaker(autocommit=False, autoflush=False, bind=commissions_engine)
    
    with commissions_engine.connect() as conn:
        print(f"Database connection (Commissions) successful to {commissions_params.get('host', 'unknown')}.")

except OperationalError as e:
    print(f"Error connecting to Commissions database: {e}")
    exit(1)

def get_commissions_db():
    db = CommissionsSessionLocal()
    try:
        yield db
    finally:
        db.close()

def get_commissions_db_session() -> Session:
    return CommissionsSessionLocal()

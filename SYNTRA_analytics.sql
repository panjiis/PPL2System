CREATE TABLE "sales_summary_daily" (
  "id" bigint PRIMARY KEY,
  "date" date NOT NULL,
  "cashier_id" bigint NOT NULL,
  "total_transactions" int DEFAULT 0,
  "total_items_sold" int DEFAULT 0,
  "gross_sales" decimal(15,2) DEFAULT 0,
  "total_discounts" decimal(15,2) DEFAULT 0,
  "net_sales" decimal(15,2) DEFAULT 0,
  "total_tax" decimal(15,2) DEFAULT 0,
  "total_cost" decimal(15,2) DEFAULT 0,
  "gross_profit" decimal(15,2) DEFAULT 0,
  "created_at" timestamp DEFAULT (now()),
  "updated_at" timestamp DEFAULT (now())
);

CREATE TABLE "product_sales_summary" (
  "id" bigint PRIMARY KEY,
  "date" date NOT NULL,
  "product_id" int NOT NULL,
  "product_group_id" int,
  "quantity_sold" int DEFAULT 0,
  "gross_sales" decimal(15,2) DEFAULT 0,
  "total_discounts" decimal(15,2) DEFAULT 0,
  "net_sales" decimal(15,2) DEFAULT 0,
  "total_cost" decimal(15,2) DEFAULT 0,
  "gross_profit" decimal(15,2) DEFAULT 0,
  "created_at" timestamp DEFAULT (now()),
  "updated_at" timestamp DEFAULT (now())
);

CREATE TABLE "employee_performance" (
  "id" bigint PRIMARY KEY,
  "date" date NOT NULL,
  "employee_id" bigint NOT NULL,
  "total_sales" decimal(15,2) DEFAULT 0,
  "total_transactions" int DEFAULT 0,
  "total_items_sold" int DEFAULT 0,
  "commission_earned" decimal(15,2) DEFAULT 0,
  "performance_score" decimal(5,2) DEFAULT 0,
  "created_at" timestamp DEFAULT (now()),
  "updated_at" timestamp DEFAULT (now())
);

CREATE TABLE "customer_analytics" (
  "id" bigint PRIMARY KEY,
  "date" date NOT NULL,
  "product_group_id" int,
  "total_transactions" int DEFAULT 0,
  "total_revenue" decimal(15,2) DEFAULT 0,
  "average_transaction_value" decimal(15,2) DEFAULT 0,
  "peak_hour" varchar,
  "created_at" timestamp DEFAULT (now()),
  "updated_at" timestamp DEFAULT (now())
);

-- 3. Buat index (juga tanpa prefix)
CREATE UNIQUE INDEX ON "sales_summary_daily" ("date", "cashier_id");
CREATE UNIQUE INDEX ON "product_sales_summary" ("date", "product_id");
CREATE UNIQUE INDEX ON "employee_performance" ("date", "employee_id");
CREATE UNIQUE INDEX ON "customer_analytics" ("date", "product_group_id");

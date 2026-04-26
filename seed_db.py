#!/usr/bin/env python3
"""
seed_db.py — Populates the Garud Kavach PostgreSQL database with realistic,
production-like seed data for FY 2025-26.

Usage:
    python seed_db.py               # Insert seed data (idempotent)
    python seed_db.py --reset       # Truncate all tables first, then seed
    python seed_db.py --dry-run     # Preview generated counts without DB writes
"""

import argparse
import json
import os
import random
import sys
from datetime import date, datetime, timedelta, timezone
from decimal import Decimal, ROUND_HALF_UP

import psycopg2
from dotenv import load_dotenv
from faker import Faker

# ──────────────────────────────────────────────────────
# CONFIG
# ──────────────────────────────────────────────────────
load_dotenv()

IST = timezone(timedelta(hours=5, minutes=30))
fake = Faker("en_IN")
Faker.seed(42)
random.seed(42)

FY_START = datetime(2025, 4, 1, tzinfo=IST)
FY_END = datetime(2026, 4, 26, tzinfo=IST)
FY_DAYS = (FY_END - FY_START).days

# Pre-computed bcrypt hash of "password123" (cost 10) — matches original Go seed_test_users.
BCRYPT_HASH = "$2a$10$zE2cRLdjnH9cJuePEvPfqe8xqHo1w.lO8lwyv08xZyZV1qAjm/TrC"

# ──────────────────────────────────────────────────────
# CONSTANTS
# ──────────────────────────────────────────────────────
CITIES = [
    "Delhi", "Mumbai", "Gurugram", "Bengaluru", "Pune",
    "Hyderabad", "Chennai", "Kolkata", "Jaipur", "Ahmedabad",
]

SERVICES = [
    "Event Security", "Residential Security", "Corporate Security",
    "ATM Security", "VIP Protection", "Industrial Security", "Retail Security",
]

GUARD_RANGES = {
    "Event Security": (5, 20),
    "Residential Security": (1, 4),
    "Corporate Security": (2, 8),
    "ATM Security": (1, 2),
    "VIP Protection": (2, 5),
    "Industrial Security": (3, 10),
    "Retail Security": (1, 4),
}

DURATION_TYPES = ["Hours", "Days", "Months"]

EXPENSE_CATEGORIES = [
    "Equipment", "Vehicle", "Salaries", "Training",
    "Maintenance", "Office Supplies", "Communication", "Insurance",
]

EXPENSE_RANGES = {
    "Equipment": (5000, 80000),
    "Vehicle": (15000, 200000),
    "Salaries": (50000, 500000),
    "Training": (3000, 30000),
    "Maintenance": (500, 20000),
    "Office Supplies": (500, 20000),
    "Communication": (500, 20000),
    "Insurance": (500, 20000),
}

EXPENSE_DESCRIPTIONS = {
    "Equipment": [
        "Purchased 10 body-worn cameras for event deployment",
        "Acquired 5 handheld metal detectors for client site in Gurugram",
        "Bought 20 guard uniforms (winter set) from vendor",
        "Procured 15 LED torches for night shift guards",
        "Ordered 8 bulletproof vests for VIP protection unit",
        "Purchased 12 safety helmets for industrial site team",
        "Acquired 6 first-aid kits for residential deployment",
        "Bought 10 reflective safety jackets for highway patrol",
    ],
    "Vehicle": [
        "Monthly lease for Mahindra Bolero patrol vehicle",
        "Fuel reimbursement for all patrol vehicles — quarterly",
        "Quarterly maintenance of Tata Ace logistics van",
        "Insurance renewal for 3 company two-wheelers",
        "Tyre replacement for Maruti Ertiga escort vehicle",
        "GPS tracker installation on 5 patrol vehicles",
    ],
    "Salaries": [
        "Guard salaries — Delhi NCR region monthly payout",
        "Guard salaries — Mumbai region monthly payout",
        "Overtime pay for Bengaluru event deployment team",
        "Monthly salary disbursement — Pune operations staff",
        "Performance bonus payout for top-rated guards Q3",
        "Salary advance disbursement for 8 field guards",
    ],
    "Training": [
        "Self-defence training workshop for 25 guards at Gurugram centre",
        "First-aid and CPR certification for 15 guards",
        "Fire safety drill session at training academy Pune",
        "Communication and client-handling soft-skills session",
        "CCTV monitoring and surveillance tech training",
        "Emergency response protocol workshop for Delhi team",
    ],
    "Maintenance": [
        "Office AC servicing at HQ Gurugram",
        "CCTV system annual maintenance contract renewal",
        "UPS battery replacement at server room",
        "Office furniture repair and refurbishment",
    ],
    "Office Supplies": [
        "Stationery and printing supplies for admin team",
        "Purchased 5 reams A4 paper and toner cartridges",
        "Guard ID card printing — batch of 50 cards",
        "Visitor log books and receipt pad replenishment",
    ],
    "Communication": [
        "Monthly Jio corporate mobile plan for 30 SIMs",
        "Walkie-talkie battery replacement — bulk order of 20",
        "Airtel broadband subscription for Pune branch office",
        "Prepaid recharges for 15 field supervisors",
    ],
    "Insurance": [
        "Annual group accident insurance premium for 60 guards",
        "Third-party liability insurance renewal — FY 2025-26",
        "Professional indemnity insurance annual premium",
        "Worker compensation insurance quarterly premium",
    ],
}

LEAVE_REASONS = [
    "Personal", "Medical", "Family Emergency",
    "Maternity/Paternity", "Vacation", "Bereavement",
]

AUDIT_ACTIONS = [
    "create_guard", "update_guard", "status_update",
    "assign_guard", "finalize_payroll", "create_invoice",
    "mark_invoice_paid", "add_expense", "approve_leave",
    "reject_leave", "create_shift", "update_shift",
]

INDIAN_MALE_FIRST = [
    "Aarav", "Vivaan", "Aditya", "Vihaan", "Arjun", "Sai", "Reyansh",
    "Ayaan", "Krishna", "Ishaan", "Rohan", "Karan", "Vikram", "Suresh",
    "Rajesh", "Amit", "Deepak", "Manoj", "Sunil", "Pankaj", "Gaurav",
    "Nikhil", "Rahul", "Mohit", "Akash", "Hemant", "Pradeep", "Vishal",
    "Ravi", "Sandeep", "Manish", "Naveen", "Ajay", "Vijay", "Dinesh",
]

INDIAN_FEMALE_FIRST = [
    "Ananya", "Saanvi", "Aanya", "Aadhya", "Priya", "Neha", "Pooja",
    "Riya", "Kavya", "Shreya", "Meera", "Sonal", "Divya", "Sunita",
    "Rekha", "Komal", "Anjali", "Swati", "Nisha", "Pallavi", "Seema",
    "Geeta", "Jyoti", "Lata", "Mamta", "Archana", "Bhavna", "Chitra",
]

INDIAN_LAST = [
    "Sharma", "Verma", "Gupta", "Singh", "Kumar", "Patel", "Reddy",
    "Nair", "Iyer", "Joshi", "Mishra", "Pandey", "Chauhan", "Yadav",
    "Tiwari", "Malhotra", "Kapoor", "Bhatia", "Saxena", "Mehta",
    "Bansal", "Aggarwal", "Thakur", "Rathore", "Deshmukh", "Patil",
    "Kulkarni", "Sinha", "Chowdhury", "Dutta",
]

# ──────────────────────────────────────────────────────
# HELPERS
# ──────────────────────────────────────────────────────

def random_fy_datetime():
    """Random datetime within FY 2025-26."""
    delta = timedelta(
        days=random.randint(0, FY_DAYS - 1),
        hours=random.randint(0, 23),
        minutes=random.randint(0, 59),
    )
    return FY_START + delta


def random_fy_date():
    """Random date within FY 2025-26."""
    return (FY_START + timedelta(days=random.randint(0, FY_DAYS - 1))).date()


def random_phone():
    """+91 9XXXXXXXXX / 8XXXXXXXXX / 7XXXXXXXXX."""
    return f"+91 {random.choice(['9','8','7'])}{random.randint(100000000, 999999999)}"


def money(lo, hi):
    """Random Decimal(10,2) in [lo, hi]."""
    return Decimal(str(round(random.uniform(lo, hi), 2)))


def indian_name(gender="M"):
    first = random.choice(INDIAN_MALE_FIRST if gender == "M" else INDIAN_FEMALE_FIRST)
    last = random.choice(INDIAN_LAST)
    return f"{first} {last}"


def staff_email(name):
    parts = name.lower().split()
    return f"{parts[0]}.{parts[1]}@rakshakshield.in"


def client_email(name):
    parts = name.lower().split()
    domain = random.choice(["gmail.com", "outlook.com", "yahoo.co.in", "hotmail.com"])
    return f"{parts[0]}.{parts[1]}@{domain}"


def payment_ref(dt):
    """Realistic UPI/NEFT reference string."""
    if random.random() < 0.5:
        return f"NEFT{dt.strftime('%Y%m')}{random.randint(1000000, 9999999)}"
    bank = random.choice(["HDFC", "ICICI", "SBI", "AXIS", "KOTAK", "PNB"])
    return f"UPI/{random.randint(200000000000, 999999999999)}/{bank}"


def table_has_data(cur, table):
    cur.execute(f"SELECT EXISTS (SELECT 1 FROM {table} LIMIT 1)")
    return cur.fetchone()[0]


# ──────────────────────────────────────────────────────
# TABLES IN FK ORDER (for truncation)
# ──────────────────────────────────────────────────────
TRUNCATE_ORDER = [
    "audit_logs",
    "leave_requests",
    "payroll",
    "shifts",
    "expenses",
    "invoices",
    "guard_query_assignments",
    "guards",
    "queries",
    "users",
]


# ══════════════════════════════════════════════════════
# SECTION SEEDERS
# ══════════════════════════════════════════════════════

def seed_users(cur):
    """Section A — Users / Admins (original test accounts + extras)."""
    if table_has_data(cur, "users"):
        print("⚠️  Section A: users table already has data — skipping")
        cur.execute("SELECT id, role, name FROM users WHERE deleted_at IS NULL ORDER BY id")
        return [{"id": r[0], "role": r[1], "name": r[2]} for r in cur.fetchall()]

    # Original test login credentials (password: password123)
    staff = [
        ("Super Admin",    "superadmin", "superadmin@test.com"),
        ("Admin User",     "superadmin", "admin@test.com"),
        ("Manager User",   "manager",    "manager@test.com"),
        ("Manager Two",    "manager",    "manager2@test.com"),
        ("Finance User",   "finance",    "finance@test.com"),
        ("Finance Two",    "finance",    "finance2@test.com"),
        ("HR User",        "hr",         "hr@test.com"),
        ("HR Two",         "hr",         "hr2@test.com"),
        ("Manager Three",  "manager",    "manager3@test.com"),
        ("Finance Three",  "finance",    "finance3@test.com"),
    ]

    users = []
    for name, role, email in staff:
        created = FY_START + timedelta(days=random.randint(0, 30))
        cur.execute(
            """INSERT INTO users (email, password, role, name, created_at, deleted_at)
               VALUES (%s, %s, %s, %s, %s, NULL) RETURNING id""",
            (email, BCRYPT_HASH, role, name, created),
        )
        uid = cur.fetchone()[0]
        users.append({"id": uid, "role": role, "name": name})

    print(f"✅ Section A: {len(users)} users inserted")
    return users


def seed_queries(cur, users):
    """Section B — Client Queries / Bookings (65 rows)."""
    if table_has_data(cur, "queries"):
        print("⚠️  Section B: queries table already has data — skipping")
        cur.execute(
            "SELECT id, status, cost, submitted_at FROM queries WHERE deleted_at IS NULL ORDER BY id"
        )
        return [{"id": r[0], "status": r[1], "cost": float(r[2]), "submitted_at": r[3]}
                for r in cur.fetchall()]

    all_user_ids = [u["id"] for u in users]
    status_weights = [0.30, 0.25, 0.30, 0.15]
    statuses = ["Pending", "In Progress", "Resolved", "Rejected"]

    queries = []
    for _ in range(65):
        service = random.choice(SERVICES)
        lo, hi = GUARD_RANGES[service]
        num_guards = random.randint(lo, hi)
        duration_type = random.choice(DURATION_TYPES)

        if duration_type == "Hours":
            duration_value = Decimal(str(random.randint(4, 24)))
        elif duration_type == "Days":
            duration_value = Decimal(str(random.randint(1, 30)))
        else:
            duration_value = Decimal(str(random.randint(1, 12)))

        rate = money(150, 400)
        if duration_type == "Hours":
            cost = Decimal(num_guards) * duration_value * rate
        elif duration_type == "Days":
            cost = Decimal(num_guards) * duration_value * rate * Decimal("8")
        else:
            cost = Decimal(num_guards) * duration_value * rate * Decimal("208")
        cost = cost.quantize(Decimal("0.01"), rounding=ROUND_HALF_UP)

        status = random.choices(statuses, weights=status_weights, k=1)[0]

        # ~30% guest submissions (user_id = NULL)
        user_id = None if random.random() < 0.30 else random.choice(all_user_ids)

        gender = random.choice(["M", "F"])
        cname = indian_name(gender)
        cemail = client_email(cname)
        phone = random_phone()
        city = random.choice(CITIES)
        venue = random.choice(["office", "residence", "event venue", "warehouse", "retail outlet", "factory", "society"])
        message = (
            f"We require {num_guards} security guard{'s' if num_guards > 1 else ''} for {service.lower()} "
            f"at our {venue} in {city}. Duration: {duration_value} {duration_type.lower()}."
        )

        is_vip = service == "VIP Protection"
        camera_req = random.random() < (0.60 if service in ("Event Security", "Corporate Security") else 0.25)
        vehicle_req = random.random() < (0.50 if is_vip else 0.20)
        first_aid = random.random() < 0.35
        walkie_talkie = random.random() < (0.80 if service in ("Event Security", "VIP Protection") else 0.30)
        bullet_proof = random.random() < 0.20 if is_vip else False
        fire_safety = random.random() < (0.50 if service == "Industrial Security" else 0.10)

        submitted_at = random_fy_datetime()

        cur.execute(
            """INSERT INTO queries
               (user_id, name, email, phone, service, message,
                num_guards, duration_type, duration_value,
                camera_required, vehicle_required, first_aid,
                walkie_talkie, bullet_proof, fire_safety,
                status, cost, submitted_at, deleted_at)
               VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,NULL)
               RETURNING id""",
            (user_id, cname, cemail, phone, service, message,
             num_guards, duration_type, duration_value,
             camera_req, vehicle_req, first_aid,
             walkie_talkie, bullet_proof, fire_safety,
             status, cost, submitted_at),
        )
        qid = cur.fetchone()[0]
        queries.append({
            "id": qid, "status": status, "cost": float(cost),
            "submitted_at": submitted_at, "service": service,
            "num_guards": num_guards,
        })

    print(f"✅ Section B: {len(queries)} client queries inserted")
    return queries


def seed_guards(cur):
    """Section C — Guards (65 rows)."""
    if table_has_data(cur, "guards"):
        print("⚠️  Section C: guards table already has data — skipping")
        cur.execute(
            "SELECT id, status, hourly_rate FROM guards WHERE deleted_at IS NULL ORDER BY id"
        )
        return [{"id": r[0], "status": r[1], "hourly_rate": float(r[2])} for r in cur.fetchall()]

    status_weights = [0.70, 0.15, 0.15]
    guard_statuses = ["active", "inactive", "on_leave"]

    guards = []
    for i in range(65):
        gender = random.choice(["M", "F"])
        name = indian_name(gender)
        phone = random_phone()
        email = client_email(name)
        city = random.choice(CITIES)
        address = f"{random.randint(1, 500)}, {fake.street_name()}, {city}"
        license_no = f"LIC-{random.randint(100000, 999999)}"
        license_expiry = date(2025, 6, 1) + timedelta(days=random.randint(0, 1310))
        status = random.choices(guard_statuses, weights=status_weights, k=1)[0]
        hourly_rate = money(120, 350)
        photo_url = f"https://res.cloudinary.com/garudkavach/image/upload/guards/guard_{i + 1}.jpg"
        created = FY_START + timedelta(days=random.randint(0, 60))

        cur.execute(
            """INSERT INTO guards
               (name, phone, email, address, license_no, license_expiry,
                status, hourly_rate, photo_url, created_at, deleted_at)
               VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,NULL)
               RETURNING id""",
            (name, phone, email, address, license_no, license_expiry,
             status, hourly_rate, photo_url, created),
        )
        gid = cur.fetchone()[0]
        guards.append({"id": gid, "status": status, "hourly_rate": float(hourly_rate),
                        "created_at": created})

    print(f"✅ Section C: {len(guards)} guards inserted")
    return guards


def seed_guard_assignments(cur, guards, queries):
    """Guard-Query Assignments — link active guards to resolved/in-progress queries."""
    if table_has_data(cur, "guard_query_assignments"):
        print("⚠️  Guard assignments already have data — skipping")
        cur.execute("SELECT id, guard_id, query_id, assigned_at FROM guard_query_assignments ORDER BY id")
        return [{"id": r[0], "guard_id": r[1], "query_id": r[2], "assigned_at": r[3]} for r in cur.fetchall()]

    eligible = [q for q in queries if q["status"] in ("Resolved", "In Progress")]
    active = [g for g in guards if g["status"] == "active"]

    assignments = []
    for guard in active:
        if not eligible:
            break
        query = random.choice(eligible)
        assigned_at = query["submitted_at"] + timedelta(hours=random.randint(1, 72))

        cur.execute(
            """INSERT INTO guard_query_assignments (guard_id, query_id, assigned_at, unassigned_at)
               VALUES (%s, %s, %s, NULL) RETURNING id""",
            (guard["id"], query["id"], assigned_at),
        )
        aid = cur.fetchone()[0]
        assignments.append({
            "id": aid, "guard_id": guard["id"],
            "query_id": query["id"], "assigned_at": assigned_at,
        })

    print(f"✅ Guard assignments: {len(assignments)} rows inserted")
    return assignments


def seed_invoices(cur, queries):
    """Section E — Finance: Invoices (55+ rows)."""
    if table_has_data(cur, "invoices"):
        print("⚠️  Section E: invoices already have data — skipping")
        cur.execute("SELECT id, query_id, status FROM invoices WHERE deleted_at IS NULL ORDER BY id")
        return [{"id": r[0], "query_id": r[1], "status": r[2]} for r in cur.fetchall()]

    billable = [q for q in queries if q["status"] in ("Resolved", "In Progress")]
    if len(billable) < 55:
        extras = [q for q in queries if q["status"] == "Pending"]
        billable += extras[: 55 - len(billable)]

    status_choices = ["paid", "pending", "refunded"]
    status_weights = [0.40, 0.45, 0.15]

    invoices = []
    for q in billable:
        inv_status = random.choices(status_choices, weights=status_weights, k=1)[0]
        issued_at = q["submitted_at"] + timedelta(days=random.randint(1, 3))
        paid_at = None
        pay_ref = None
        if inv_status == "paid":
            paid_at = issued_at + timedelta(days=random.randint(2, 10))
            pay_ref = payment_ref(paid_at)

        cur.execute(
            """INSERT INTO invoices
               (query_id, amount, status, issued_at, paid_at, payment_ref, deleted_at)
               VALUES (%s,%s,%s,%s,%s,%s,NULL) RETURNING id""",
            (q["id"], q["cost"], inv_status, issued_at, paid_at, pay_ref),
        )
        iid = cur.fetchone()[0]
        invoices.append({"id": iid, "query_id": q["id"], "status": inv_status,
                         "issued_at": issued_at, "paid_at": paid_at})

    print(f"✅ Section E: {len(invoices)} invoices inserted")
    return invoices


def seed_expenses(cur, users):
    """Section F — Finance: Expenses (65 rows)."""
    if table_has_data(cur, "expenses"):
        print("⚠️  Section F: expenses already have data — skipping")
        cur.execute("SELECT id FROM expenses WHERE deleted_at IS NULL")
        return [{"id": r[0]} for r in cur.fetchall()]

    fin_ids = [u["id"] for u in users if u["role"] in ("finance", "superadmin", "manager")]
    if not fin_ids:
        fin_ids = [users[0]["id"]]

    expenses = []
    for _ in range(65):
        category = random.choice(EXPENSE_CATEGORIES)
        lo, hi = EXPENSE_RANGES[category]
        amount = money(lo, hi)
        description = random.choice(EXPENSE_DESCRIPTIONS[category])
        exp_date = random_fy_date()
        added_by = random.choice(fin_ids)

        cur.execute(
            """INSERT INTO expenses
               (category, description, amount, expense_date, added_by, deleted_at)
               VALUES (%s,%s,%s,%s,%s,NULL) RETURNING id""",
            (category, description, amount, exp_date, added_by),
        )
        eid = cur.fetchone()[0]
        expenses.append({"id": eid})

    print(f"✅ Section F: {len(expenses)} expenses inserted")
    return expenses


def seed_shifts(cur, guards, assignments, queries):
    """Section G — HR: Shifts (130+ rows)."""
    if table_has_data(cur, "shifts"):
        print("⚠️  Section G: shifts already have data — skipping")
        cur.execute(
            "SELECT id, guard_id, actual_hours, status, start_time "
            "FROM shifts WHERE deleted_at IS NULL ORDER BY id"
        )
        return [{"id": r[0], "guard_id": r[1], "actual_hours": float(r[2] or 0),
                 "status": r[3], "start_time": r[4]} for r in cur.fetchall()]

    guard_query_map = {}
    for a in assignments:
        guard_query_map.setdefault(a["guard_id"], []).append(a["query_id"])

    shift_patterns = [
        (6, 0, 14, 0, 8.0),    # morning
        (14, 0, 22, 0, 8.0),   # afternoon
        (22, 0, 6, 0, 8.0),    # night
    ]
    status_choices = ["scheduled", "active", "completed", "cancelled"]
    status_weights = [0.20, 0.10, 0.60, 0.10]
    active_guards = [g for g in guards if g["status"] == "active"]

    shifts = []
    for guard in active_guards:
        num = random.randint(2, 5)
        qids = guard_query_map.get(guard["id"], [])
        for _ in range(num):
            query_id = random.choice(qids) if qids else None
            pat = random.choice(shift_patterns)
            sd = random_fy_date()
            start_time = datetime(sd.year, sd.month, sd.day, pat[0], pat[1], tzinfo=IST)
            end_time = start_time + timedelta(hours=int(pat[4]))
            actual_hours = Decimal(str(pat[4]))
            shift_status = random.choices(status_choices, weights=status_weights, k=1)[0]

            cur.execute(
                """INSERT INTO shifts
                   (guard_id, query_id, start_time, end_time, actual_hours, status, deleted_at)
                   VALUES (%s,%s,%s,%s,%s,%s,NULL) RETURNING id""",
                (guard["id"], query_id, start_time, end_time, actual_hours, shift_status),
            )
            sid = cur.fetchone()[0]
            shifts.append({"id": sid, "guard_id": guard["id"],
                           "actual_hours": float(actual_hours), "status": shift_status,
                           "start_time": start_time})

    # Pad to ≥ 120 if needed
    while len(shifts) < 130:
        guard = random.choice(active_guards)
        qids = guard_query_map.get(guard["id"], [])
        query_id = random.choice(qids) if qids else None
        pat = random.choice(shift_patterns)
        sd = random_fy_date()
        start_time = datetime(sd.year, sd.month, sd.day, pat[0], pat[1], tzinfo=IST)
        end_time = start_time + timedelta(hours=int(pat[4]))
        actual_hours = Decimal(str(pat[4]))
        shift_status = random.choices(status_choices, weights=status_weights, k=1)[0]

        cur.execute(
            """INSERT INTO shifts
               (guard_id, query_id, start_time, end_time, actual_hours, status, deleted_at)
               VALUES (%s,%s,%s,%s,%s,%s,NULL) RETURNING id""",
            (guard["id"], query_id, start_time, end_time, actual_hours, shift_status),
        )
        sid = cur.fetchone()[0]
        shifts.append({"id": sid, "guard_id": guard["id"],
                       "actual_hours": float(actual_hours), "status": shift_status,
                       "start_time": start_time})

    print(f"✅ Section G: {len(shifts)} shifts inserted")
    return shifts


def seed_payroll(cur, guards, shifts):
    """Section H — HR: Payroll (60+ rows)."""
    if table_has_data(cur, "payroll"):
        print("⚠️  Section H: payroll already has data — skipping")
        cur.execute("SELECT id, guard_id FROM payroll WHERE deleted_at IS NULL")
        return [{"id": r[0], "guard_id": r[1]} for r in cur.fetchall()]

    # Aggregate completed shift hours per guard per month
    guard_monthly = {}
    for s in shifts:
        if s["status"] != "completed":
            continue
        month_key = s["start_time"].strftime("%Y-%m")
        key = (s["guard_id"], month_key)
        guard_monthly[key] = guard_monthly.get(key, 0.0) + s["actual_hours"]

    guard_rate = {g["id"]: g["hourly_rate"] for g in guards}
    pay_statuses = ["paid", "pending"]
    pay_weights = [0.60, 0.40]

    payroll = []
    for (guard_id, month_str), total_hours in guard_monthly.items():
        rate = Decimal(str(guard_rate.get(guard_id, 200)))
        hrs = Decimal(str(total_hours)).quantize(Decimal("0.01"), rounding=ROUND_HALF_UP)
        total_pay = (hrs * rate).quantize(Decimal("0.01"), rounding=ROUND_HALF_UP)
        month_date = datetime.strptime(month_str, "%Y-%m").date().replace(day=1)
        status = random.choices(pay_statuses, weights=pay_weights, k=1)[0]
        paid_at = None
        if status == "paid":
            next_m = month_date + timedelta(days=35)
            paid_at = datetime(next_m.year, next_m.month, random.randint(1, 5), tzinfo=IST)

        cur.execute(
            """INSERT INTO payroll
               (guard_id, month, total_hours, rate_per_hour, total_pay, status, paid_at, deleted_at)
               VALUES (%s,%s,%s,%s,%s,%s,%s,NULL) RETURNING id""",
            (guard_id, month_date, hrs, rate, total_pay, status, paid_at),
        )
        pid = cur.fetchone()[0]
        payroll.append({"id": pid, "guard_id": guard_id})

    # Pad to ≥ 60 with synthetic rows
    active_guards = [g for g in guards if g["status"] == "active"]
    fy_months = []
    d = date(2025, 4, 1)
    while d <= date(2026, 3, 1):
        fy_months.append(d)
        if d.month == 12:
            d = date(d.year + 1, 1, 1)
        else:
            d = d.replace(month=d.month + 1)

    while len(payroll) < 65:
        guard = random.choice(active_guards)
        month_date = random.choice(fy_months)
        rate = Decimal(str(guard["hourly_rate"]))
        hrs = Decimal(str(random.randint(40, 200))).quantize(Decimal("0.01"))
        total_pay = (hrs * rate).quantize(Decimal("0.01"), rounding=ROUND_HALF_UP)
        status = random.choices(pay_statuses, weights=pay_weights, k=1)[0]
        paid_at = None
        if status == "paid":
            next_m = month_date + timedelta(days=35)
            paid_at = datetime(next_m.year, next_m.month, random.randint(1, 5), tzinfo=IST)

        cur.execute(
            """INSERT INTO payroll
               (guard_id, month, total_hours, rate_per_hour, total_pay, status, paid_at, deleted_at)
               VALUES (%s,%s,%s,%s,%s,%s,%s,NULL) RETURNING id""",
            (guard["id"], month_date, hrs, rate, total_pay, status, paid_at),
        )
        pid = cur.fetchone()[0]
        payroll.append({"id": pid, "guard_id": guard["id"]})

    print(f"✅ Section H: {len(payroll)} payroll records inserted")
    return payroll


def seed_leave_requests(cur, guards, users, shifts):
    """Section I — HR: Leave Requests (55+ rows)."""
    if table_has_data(cur, "leave_requests"):
        print("⚠️  Section I: leave_requests already has data — skipping")
        cur.execute("SELECT id, guard_id, status FROM leave_requests WHERE deleted_at IS NULL")
        return [{"id": r[0], "guard_id": r[1], "status": r[2]} for r in cur.fetchall()]

    hr_ids = [u["id"] for u in users if u["role"] in ("hr", "superadmin", "manager")]
    if not hr_ids:
        hr_ids = [users[0]["id"]]

    # Build busy-date sets from active/scheduled shifts
    busy_dates = {}
    for s in shifts:
        if s["status"] in ("active", "scheduled") and isinstance(s["start_time"], datetime):
            busy_dates.setdefault(s["guard_id"], set()).add(s["start_time"].date())

    all_guard_ids = [g["id"] for g in guards]
    status_choices = ["approved", "pending", "rejected"]
    status_weights = [0.40, 0.35, 0.25]

    leaves = []
    attempts = 0
    while len(leaves) < 55 and attempts < 400:
        attempts += 1
        guard_id = random.choice(all_guard_ids)
        duration_days = random.randint(1, 14)
        start_date = random_fy_date()
        end_date = start_date + timedelta(days=duration_days - 1)
        if end_date > FY_END.date():
            continue

        guard_busy = busy_dates.get(guard_id, set())
        overlap = any(
            (start_date + timedelta(days=d)) in guard_busy
            for d in range(duration_days)
        )
        if overlap:
            continue

        reason = random.choice(LEAVE_REASONS)
        leave_status = random.choices(status_choices, weights=status_weights, k=1)[0]
        reviewed_by = random.choice(hr_ids) if leave_status in ("approved", "rejected") else None

        created_at = datetime(start_date.year, start_date.month, start_date.day,
                              tzinfo=IST) - timedelta(days=random.randint(1, 7))
        if created_at < FY_START:
            created_at = FY_START

        cur.execute(
            """INSERT INTO leave_requests
               (guard_id, start_date, end_date, reason, status, reviewed_by, created_at, deleted_at)
               VALUES (%s,%s,%s,%s,%s,%s,%s,NULL) RETURNING id""",
            (guard_id, start_date, end_date, reason, leave_status, reviewed_by, created_at),
        )
        lid = cur.fetchone()[0]
        leaves.append({"id": lid, "guard_id": guard_id, "status": leave_status})

    print(f"✅ Section I: {len(leaves)} leave requests inserted")
    return leaves


def seed_audit_logs(cur, users, guards, queries, invoices, shifts, leaves):
    """Section D — Audit Logs (85 rows)."""
    if table_has_data(cur, "audit_logs"):
        print("⚠️  Section D: audit_logs already has data — skipping")
        return

    staff_ids = [u["id"] for u in users]
    guard_ids = [g["id"] for g in guards]
    query_ids = [q["id"] for q in queries]
    invoice_ids = [i["id"] for i in invoices]
    shift_ids = [s["id"] for s in shifts]
    leave_ids = [l["id"] for l in leaves]

    def _detail(action, tid):
        if action == "create_guard":
            return {"name": indian_name(), "status": "active", "city": random.choice(CITIES)}
        if action == "update_guard":
            return {"field": random.choice(["phone", "address", "hourly_rate"]),
                    "old_value": "previous", "new_value": "updated"}
        if action == "status_update":
            return {"entity": "query", "id": tid,
                    "old_status": "Pending",
                    "new_status": random.choice(["In Progress", "Resolved", "Rejected"])}
        if action == "assign_guard":
            return {"guard_id": random.choice(guard_ids), "query_id": random.choice(query_ids)}
        if action == "finalize_payroll":
            return {"month": random.choice(["2025-05", "2025-08", "2025-11", "2026-02"]),
                    "guards_processed": random.randint(5, 30)}
        if action == "create_invoice":
            return {"query_id": random.choice(query_ids),
                    "amount": float(money(5000, 200000))}
        if action == "mark_invoice_paid":
            return {"invoice_id": tid,
                    "payment_ref": payment_ref(random_fy_datetime())}
        if action == "add_expense":
            return {"category": random.choice(EXPENSE_CATEGORIES),
                    "amount": float(money(500, 50000))}
        if action == "approve_leave":
            return {"guard_id": random.choice(guard_ids), "days": random.randint(1, 14),
                    "reason": random.choice(LEAVE_REASONS)}
        if action == "reject_leave":
            return {"guard_id": random.choice(guard_ids),
                    "reason": "Insufficient notice period"}
        if action == "create_shift":
            return {"guard_id": random.choice(guard_ids),
                    "type": random.choice(["morning", "afternoon", "night"])}
        if action == "update_shift":
            return {"shift_id": tid, "field": "end_time", "change": "+2 hours"}
        return {}

    count = 0
    for _ in range(85):
        action = random.choice(AUDIT_ACTIONS)
        user_id = random.choice(staff_ids)

        if action in ("create_guard", "update_guard"):
            tid = random.choice(guard_ids)
            target = f"guard:{tid}"
        elif action in ("status_update", "assign_guard"):
            tid = random.choice(query_ids)
            target = f"query:{tid}"
        elif action in ("create_invoice", "mark_invoice_paid"):
            tid = random.choice(invoice_ids)
            target = f"invoice:{tid}"
        elif action in ("create_shift", "update_shift"):
            tid = random.choice(shift_ids)
            target = f"shift:{tid}"
        elif action in ("approve_leave", "reject_leave"):
            tid = random.choice(leave_ids)
            target = f"leave:{tid}"
        elif action == "finalize_payroll":
            tid = random.randint(1, 20)
            target = f"payroll:{tid}"
        elif action == "add_expense":
            tid = random.randint(1, 60)
            target = f"expense:{tid}"
        else:
            tid = 1
            target = f"entity:{tid}"

        details = json.dumps(_detail(action, tid))
        created_at = random_fy_datetime()

        cur.execute(
            """INSERT INTO audit_logs
               (user_id, action, target, details, created_at, deleted_at)
               VALUES (%s,%s,%s,%s::jsonb,%s,NULL)""",
            (user_id, action, target, details, created_at),
        )
        count += 1

    print(f"✅ Section D: {count} audit log entries inserted")


# ══════════════════════════════════════════════════════
# MAIN
# ══════════════════════════════════════════════════════

def get_connection():
    """Connect using DATABASE_URL or individual DB_* vars."""
    db_url = os.getenv("DATABASE_URL")
    if db_url:
        return psycopg2.connect(db_url)

    host = os.getenv("DB_HOST", "localhost")
    port = int(os.getenv("DB_PORT", 5432))
    dbname = os.getenv("DB_NAME", "garudkavach")
    user = os.getenv("DB_USER", "postgres")
    password = os.getenv("DB_PASSWORD", "")
    return psycopg2.connect(host=host, port=port, dbname=dbname, user=user, password=password)


def main():
    parser = argparse.ArgumentParser(description="Seed the Garud Kavach database")
    parser.add_argument("--reset", action="store_true",
                        help="TRUNCATE all tables in reverse FK order before seeding")
    parser.add_argument("--dry-run", action="store_true",
                        help="Generate data counts without touching the database")
    args = parser.parse_args()

    if args.dry_run:
        print("=== DRY RUN — no database connection ===")
        print(f"Would generate:")
        print(f"  Users          : 10")
        print(f"  Queries        : 65")
        print(f"  Guards         : 65")
        print(f"  Assignments    : ~45 (active guards)")
        print(f"  Invoices       : 55+")
        print(f"  Expenses       : 65")
        print(f"  Shifts         : 130+")
        print(f"  Payroll        : 65+")
        print(f"  Leave Requests : 55+")
        print(f"  Audit Logs     : 85")
        print(f"\nDate range: {FY_START.date()} → {FY_END.date()}")
        print("Dry run complete. No data was inserted.")
        return

    try:
        conn = get_connection()
        conn.autocommit = False
    except psycopg2.OperationalError as e:
        print(f"❌ Could not connect to database: {e}")
        sys.exit(1)

    cur = conn.cursor()

    # ── RESET ──
    if args.reset:
        print("🔄 Resetting all tables …")
        for table in TRUNCATE_ORDER:
            cur.execute(f"TRUNCATE TABLE {table} RESTART IDENTITY CASCADE")
        conn.commit()
        print("✅ All tables truncated\n")

    # ── SEED (each section in its own transaction) ──
    sections = [
        ("Section A (Users)", lambda: seed_users(cur)),
    ]

    # Run Users first
    try:
        users = seed_users(cur)
        conn.commit()
    except Exception as e:
        conn.rollback()
        print(f"❌ Section A failed: {e}")
        cur.close(); conn.close()
        sys.exit(1)

    try:
        queries = seed_queries(cur, users)
        conn.commit()
    except Exception as e:
        conn.rollback()
        print(f"❌ Section B failed: {e}")
        cur.close(); conn.close()
        sys.exit(1)

    try:
        guards = seed_guards(cur)
        conn.commit()
    except Exception as e:
        conn.rollback()
        print(f"❌ Section C failed: {e}")
        cur.close(); conn.close()
        sys.exit(1)

    try:
        assignments = seed_guard_assignments(cur, guards, queries)
        conn.commit()
    except Exception as e:
        conn.rollback()
        print(f"❌ Guard assignments failed: {e}")
        cur.close(); conn.close()
        sys.exit(1)

    try:
        invoices = seed_invoices(cur, queries)
        conn.commit()
    except Exception as e:
        conn.rollback()
        print(f"❌ Section E failed: {e}")
        cur.close(); conn.close()
        sys.exit(1)

    try:
        seed_expenses(cur, users)
        conn.commit()
    except Exception as e:
        conn.rollback()
        print(f"❌ Section F failed: {e}")
        cur.close(); conn.close()
        sys.exit(1)

    try:
        shifts = seed_shifts(cur, guards, assignments, queries)
        conn.commit()
    except Exception as e:
        conn.rollback()
        print(f"❌ Section G failed: {e}")
        cur.close(); conn.close()
        sys.exit(1)

    try:
        seed_payroll(cur, guards, shifts)
        conn.commit()
    except Exception as e:
        conn.rollback()
        print(f"❌ Section H failed: {e}")
        cur.close(); conn.close()
        sys.exit(1)

    try:
        leaves = seed_leave_requests(cur, guards, users, shifts)
        conn.commit()
    except Exception as e:
        conn.rollback()
        print(f"❌ Section I failed: {e}")
        cur.close(); conn.close()
        sys.exit(1)

    try:
        seed_audit_logs(cur, users, guards, queries, invoices, shifts, leaves)
        conn.commit()
    except Exception as e:
        conn.rollback()
        print(f"❌ Section D failed: {e}")
        cur.close(); conn.close()
        sys.exit(1)

    # ── RESET SEQUENCES ──
    print("\nResetting ID sequences …")
    seq_tables = ["users", "queries", "guards", "guard_query_assignments",
                  "invoices", "expenses", "shifts", "payroll", "leave_requests", "audit_logs"]
    for t in seq_tables:
        try:
            cur.execute(
                f"SELECT setval(pg_get_serial_sequence('{t}', 'id'), "
                f"COALESCE(MAX(id), 1)) FROM {t}"
            )
        except Exception:
            conn.rollback()
    conn.commit()

    cur.close()
    conn.close()
    print("\n🎉 Seeding complete!")


if __name__ == "__main__":
    main()

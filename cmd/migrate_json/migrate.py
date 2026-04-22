import os
import json
import psycopg2
from dotenv import load_dotenv
from datetime import datetime
import re

def parse_int_safe(value, default=1):
    try:
        return int(value)
    except (ValueError, TypeError):
        return default

def parse_float_safe(value, default=0.0):
    try:
        return float(value)
    except (ValueError, TypeError):
        return default

def parse_datetime_safe(value):
    if not value:
        return datetime.now().isoformat()
    try:
        # Attempt to parse standard RFC3339
        datetime.fromisoformat(value.replace('Z', '+00:00'))
        return value
    except ValueError:
        return datetime.now().isoformat()

def main():
    # Resolve paths relative to this script (2 levels up to the project root)
    project_root = os.path.abspath(os.path.join(os.path.dirname(__file__), '../../'))
    
    load_dotenv(dotenv_path=os.path.join(project_root, '.env'))

    database_url = os.getenv("DATABASE_URL")
    if not database_url:
        print("Error: DATABASE_URL not found in .env file")
        return

    try:
        conn = psycopg2.connect(database_url)
        cur = conn.cursor()
        # Clear the table before inserting
        cur.execute("TRUNCATE TABLE queries RESTART IDENTITY;")
        print("Cleared the 'queries' table.")
    except Exception as e:
        print(f"Error connecting to database or clearing table: {e}")
        return

    try:
        with open(os.path.join(project_root, 'database.json'), 'r') as f:
            records = json.load(f)
    except Exception as e:
        print(f"Error reading database.json: {e}")
        cur.close()
        conn.close()
        return

    inserted_count = 0
    insert_query = """
    INSERT INTO queries (
        id, name, email, phone, service, message, num_guards, duration_type,
        duration_value, camera_required, vehicle_required, first_aid,
        walkie_talkie, bullet_proof, fire_safety, status, cost, submitted_at
    ) VALUES (
        %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s
    )
    """

    for record in records:
        try:
            cur.execute(insert_query, (
                record.get('id'),
                record.get('name'),
                record.get('email'),
                record.get('phone'),
                record.get('service'),
                record.get('message'),
                parse_int_safe(record.get('numGuards')),
                record.get('durationType'),
                parse_float_safe(record.get('durationValue')),
                record.get('cameraRequired', False),
                record.get('vehicleRequired', False),
                record.get('firstAid', False),
                record.get('walkieTalkie', False),
                record.get('bulletProof', False),
                record.get('fireSafety', False),
                record.get('status', 'Pending'),
                parse_float_safe(record.get('cost')),
                parse_datetime_safe(record.get('submitted_at'))
            ))
            inserted_count += 1
        except Exception as e:
            print(f"Error inserting record ID {record.get('id')}: {e}")
            conn.rollback()
            cur.close()
            conn.close()
            return
            
    try:
        # Reset the primary key sequence
        cur.execute("SELECT setval(pg_get_serial_sequence('queries', 'id'), COALESCE((SELECT MAX(id) FROM queries), 1), true)")
        conn.commit()
    except Exception as e:
        print(f"Error updating sequence: {e}")
        conn.rollback()


    cur.close()
    conn.close()

    print(f"Records in JSON: {len(records)}")
    print(f"Records inserted: {inserted_count}")

    if inserted_count == len(records):
        print("Migration completed successfully.")
    else:
        print("Migration failed: Mismatch in record counts.")

if __name__ == "__main__":
    main()

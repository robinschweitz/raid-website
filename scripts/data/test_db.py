import sqlite3

conn = sqlite3.connect('F:/raid-website/scripts/data/azerothcore.db')
cursor = conn.cursor()
cursor.execute("SELECT name FROM sqlite_master WHERE type='table'")
tables = cursor.fetchall()
print('Tables:', [t[0] for t in tables])
for t in tables:
    cursor.execute(f'SELECT COUNT(*) FROM "{t[0]}"')
    count = cursor.fetchone()[0]
    print(f'  {t[0]}: {count} rows')
conn.close()

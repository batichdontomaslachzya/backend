"""Generate missing local secrets, preserving existing values without printing them."""
from pathlib import Path
import secrets

path = Path(__file__).resolve().parents[1] / '.env'
content = path.read_text(encoding='utf-8') if path.exists() else ''
keys = {line.split('=', 1)[0].strip() for line in content.splitlines() if '=' in line}
missing = [key for key in ('INTERNAL_TOKEN', 'POSTGRES_PASSWORD', 'REDIS_PASSWORD', 'GRAFANA_PASSWORD')
           if key not in keys]
if missing:
    with path.open('a', encoding='utf-8') as file:
        if content and not content.endswith('\n'):
            file.write('\n')
        for key in missing:
            file.write(key + '=' + secrets.token_hex(32) + '\n')
    print('Created missing local secrets in .env; existing values preserved.')
else:
    print('.env already contains all required keys.')

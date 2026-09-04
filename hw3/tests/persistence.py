"""Integration checks for HW4; temporarily restarts this local Compose stack."""
import base64
import hashlib
import json
import subprocess
import time
import urllib.parse
import urllib.request
import uuid

import smoke as api


def compose(*args):
    result = subprocess.run(['docker', 'compose', *args], cwd=api.ROOT,
                            check=True, stdout=subprocess.PIPE, text=True)
    return result.stdout.strip()


def sql(query):
    return compose('exec', '-T', 'postgres', 'psql', '-U', 'images', '-d', 'images', '-Atc', query)


def redis(*args):
    return compose('exec', '-T', 'redis', 'redis-cli', '--raw', *args)


def fetch(url, headers=None):
    with urllib.request.urlopen(urllib.request.Request(url, headers=headers or {}), timeout=10) as response:
        return json.load(response)


def query(expression):
    return fetch('http://127.0.0.1:9090/api/v1/query?'+urllib.parse.urlencode({'query': expression}))['data']['result']


def main():
    credentials = {'username': 'persist_'+uuid.uuid4().hex[:12], 'password': 'learning-password'}
    api.expect('POST', '/register', 201, body=credentials)
    login, _ = api.expect('POST', '/login', 200, body=credentials)
    token = login['token']
    source = api.png(2, 2, [(10, 20, 30, 255)]*4)
    task_id = api.upload(token, source, {'name': 'negative'})
    api.wait_task(token, task_id)
    before, _ = api.expect('GET', '/result/'+task_id, 200, token=token)
    key = 'session:'+hashlib.sha256(token.encode()).hexdigest()
    assert 0 < int(redis('TTL', key)) <= 86400
    assert sql('SELECT count(*) FROM schema_migrations') == '1'

    try:
        compose('stop', 'processor', 'api', 'postgres', 'redis')
        compose('up', '-d', '--wait', '--wait-timeout', '240', 'api', 'processor')
        after, _ = api.expect('GET', '/result/'+task_id, 200, token=token)
        assert before == after
        api.expect('POST', '/login', 200, body=credentials)
        assert sql('SELECT count(*) FROM schema_migrations') == '1'
        print('PASS: user, task, result and session survived restarts; migration applied once.', flush=True)

        compose('stop', 'redis')
        api.expect('GET', '/status/'+task_id, 503, token=token)
        compose('up', '-d', '--wait', 'redis')
        api.expect('GET', '/status/'+task_id, 200, token=token)
        compose('stop', 'postgres')
        api.expect('GET', '/status/'+task_id, 503, token=token)
        api.expect('POST', '/login', 503, body=credentials)
        compose('up', '-d', '--wait', 'postgres')
        print('PASS: storage outages return 503 and recover.', flush=True)

        compose('stop', 'processor', 'kafka')
        pending = api.upload(token, source, {'name': 'negative'})
        assert sql("SELECT published_at IS NULL FROM tasks WHERE id='"+pending+"'") == 't'
        api.expect('GET', '/result/'+pending, 202, token=token)
        compose('stop', 'api')
        compose('up', '-d', '--wait', '--wait-timeout', '240', 'api', 'processor')
        api.wait_task(token, pending)
        api.expect('GET', '/result/'+pending, 200, token=token)
        print('PASS: task accepted without Kafka survived API restart and completed.', flush=True)

        # Populate every metric label after the worker restart.
        for name in ('negative', 'flip_x', 'blur', 'sharpen'):
            filter = {'name': name}
            if name in ('blur', 'sharpen'):
                filter['parameters'] = {'sigma': 1.2}
            api.wait_task(token, api.upload(token, source, filter))
        api.wait_task(token, api.upload(token, source[:33], {'name': 'negative'}), 'failed')
        deadline = time.monotonic()+60
        while True:
            healthy = query('sum(up{job=~"api|processor"})')
            filters = query('count(sum by(filter) (image_processing_total))')
            failed = query('sum(image_processing_total{status="failed"})')
            if healthy and float(healthy[0]['value'][1]) == 2 and filters and float(filters[0]['value'][1]) == 4 and failed and float(failed[0]['value'][1]) >= 1:
                break
            assert time.monotonic() < deadline, 'Prometheus did not collect expected metrics'
            time.sleep(1)
        assert query('sum(image_processing_duration_seconds_count)')
        assert query('sum(image_api_requests_total)')
        secrets = dict(line.split('=', 1) for line in (api.ROOT/'.env').read_text().splitlines()
                       if '=' in line and not line.startswith('#'))
        auth = base64.b64encode(('admin:'+secrets['GRAFANA_PASSWORD']).encode()).decode()
        dashboard = fetch('http://127.0.0.1:3000/api/dashboards/uid/image-service', {'Authorization': 'Basic '+auth})
        assert len(dashboard['dashboard']['panels']) == 8
        assert fetch('http://127.0.0.1:3000/api/health')['database'] == 'ok'
        print('PASS: Prometheus scraped API and processor, all filters, failures and duration; Grafana dashboard provisioned.', flush=True)

        redis('PEXPIRE', key, '1')
        time.sleep(.05)
        api.expect('GET', '/status/'+task_id, 401, token=token)
        print('PASS: expired Redis session returns 401.', flush=True)
    finally:
        compose('up', '-d', '--wait', '--wait-timeout', '240')
    print('PASS: HW4 persistence, outbox, failure recovery, TTL and monitoring verified.')


if __name__ == '__main__':
    main()

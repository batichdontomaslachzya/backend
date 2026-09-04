"""End-to-end checks against the running Compose stack; Python standard library only."""
import argparse
import io
import json
import os
from pathlib import Path
import struct
import subprocess
import time
import urllib.error
import urllib.request
import uuid
import zlib

BASE = os.getenv('BASE_URL', 'http://127.0.0.1:8000')
ROOT = Path(__file__).resolve().parents[1]
checks = 0

def request(method, path, body=None, token=None, content_type='application/json'):
    headers = {'Content-Type': content_type}
    if token is not None:
        headers['Authorization'] = 'Bearer ' + token
    if isinstance(body, dict):
        body = json.dumps(body).encode()
    req = urllib.request.Request(BASE + path, data=body, method=method, headers=headers)
    try:
        response = urllib.request.urlopen(req, timeout=35)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        data = response.read()
        if response.headers.get('Content-Type', '').startswith('application/json'):
            data = json.loads(data)
        return response.status, data, response.headers

def expect(method, path, status, **kwargs):
    global checks
    actual, data, headers = request(method, path, **kwargs)
    assert actual == status, (method, path, 'expected', status, 'received', actual)
    checks += 1
    return data, headers

def png(width, height, pixels):
    def chunk(kind, data):
        return struct.pack('>I', len(data)) + kind + data + struct.pack('>I', zlib.crc32(kind + data))
    scanlines = b''.join(b'\0' + bytes(sum(pixels[y*width:(y+1)*width], ())) for y in range(height))
    return (b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('>IIBBBBB', width, height, 8, 6, 0, 0, 0))
            + chunk(b'IDAT', zlib.compress(scanlines)) + chunk(b'IEND', b''))

def decode_png(data):
    assert data[:8] == b'\x89PNG\r\n\x1a\n'
    offset, compressed = 8, b''
    while offset < len(data):
        size = struct.unpack('>I', data[offset:offset+4])[0]
        kind, part = data[offset+4:offset+8], data[offset+8:offset+8+size]
        if kind == b'IHDR':
            width, height, depth, color, _, _, interlace = struct.unpack('>IIBBBBB', part)
            assert depth == 8 and color in (2, 6) and interlace == 0
        if kind == b'IDAT':
            compressed += part
        offset += 12 + size
    channels = 4 if color == 6 else 3
    raw, stride = zlib.decompress(compressed), width*channels
    previous, pixels = bytearray(stride), []
    for y in range(height):
        method = raw[y*(stride+1)]
        row = bytearray(raw[y*(stride+1)+1:(y+1)*(stride+1)])
        for x in range(stride):
            left = row[x-channels] if x >= channels else 0
            up = previous[x]
            upper_left = previous[x-channels] if x >= channels else 0
            if method == 1: prediction = left
            elif method == 2: prediction = up
            elif method == 3: prediction = (left+up)//2
            elif method == 4:
                estimate = left+up-upper_left
                prediction = min((left, up, upper_left), key=lambda n: abs(estimate-n))
            else:
                assert method == 0
                prediction = 0
            row[x] = (row[x]+prediction) & 255
        for x in range(0, stride, channels):
            pixel = tuple(row[x:x+channels])
            pixels.append(pixel if channels == 4 else pixel+(255,))
        previous = row
    return width, height, pixels

def upload(token, image, filter, expected=201):
    boundary = 'image-test-' + uuid.uuid4().hex
    body = (f'--{boundary}\r\nContent-Disposition: form-data; name="filter"\r\n\r\n'.encode()
            + json.dumps(filter).encode()
            + f'\r\n--{boundary}\r\nContent-Disposition: form-data; name="image"; filename="example.png"\r\nContent-Type: image/png\r\n\r\n'.encode()
            + image + f'\r\n--{boundary}--\r\n'.encode())
    data, _ = expect('POST', '/task', expected, body=body, token=token,
                     content_type='multipart/form-data; boundary=' + boundary)
    return data.get('task_id')

def wait_task(token, task_id, wanted='ready'):
    deadline = time.monotonic()+60
    while time.monotonic() < deadline:
        status, data, _ = request('GET', '/status/'+task_id, token=token)
        assert status == 200
        if data['status'] != 'in_progress':
            assert data['status'] == wanted, data
            return
        time.sleep(.2)
    raise AssertionError('task did not finish within 60 seconds')

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--restart-processor', action='store_true')
    args = parser.parse_args()
    expect('GET', '/healthz', 200)
    spec, _ = expect('GET', '/openapi.yaml', 200)
    assert b'Image Service - HW4' in spec
    expect('POST', '/task', 401)
    expect('POST', '/commit', 403, body={})
    tokens = []
    for prefix in ['alice', 'bob']:
        credentials = {'username': prefix+'_'+uuid.uuid4().hex[:10], 'password': 'learning-password'}
        expect('POST', '/register', 201, body=credentials)
        expect('POST', '/register', 409, body=credentials)
        login, _ = expect('POST', '/login', 200, body=credentials)
        tokens.append(login['token'])
        expect('POST', '/login', 401, body={**credentials, 'password': 'wrong-password'})
    owner, other = tokens
    expect('POST', '/commit', 403, body={}, token=owner)
    pixels = [(10, 20, 30, 255), (200, 60, 80, 128), (0, 100, 250, 255), (240, 210, 190, 255)]
    source = png(2, 2, pixels)
    for name in ['negative', 'flip_x', 'blur', 'sharpen']:
        filter = {'name': name}
        if name in ('blur', 'sharpen'): filter['parameters'] = {'sigma': 1.2}
        task_id = upload(owner, source, filter)
        wait_task(owner, task_id)
        original, _ = expect('GET', '/image/'+task_id, 200, token=owner)
        assert original == source
        result, headers = expect('GET', '/result/'+task_id, 200, token=owner)
        assert headers['Content-Type'] == 'image/png' and headers['Cache-Control'] == 'no-store'
        width, height, actual = decode_png(result)
        assert (width, height) == (2, 2)
        if name == 'negative': assert actual == [(255-r, 255-g, 255-b, a) for r,g,b,a in pixels]
        elif name == 'flip_x': assert actual == pixels[2:]+pixels[:2]
        else: assert actual != pixels
        for route in ['image', 'status', 'result']:
            expect('GET', '/'+route+'/'+task_id, 404, token=other)
            expect('GET', '/'+route+'/'+task_id, 401, token='invalid-token')

    jpeg = (ROOT / 'tests/fixtures/sample.jpg').read_bytes()
    jpeg_id = upload(owner, jpeg, {'name': 'negative'})
    wait_task(owner, jpeg_id)
    original, headers = expect('GET', '/image/'+jpeg_id, 200, token=owner)
    assert original == jpeg and headers['Content-Type'] == 'image/jpeg'
    result, headers = expect('GET', '/result/'+jpeg_id, 200, token=owner)
    assert headers['Content-Type'] == 'image/png' and decode_png(result)[:2] == (2, 2)

    # A valid PNG with a large ancillary chunk exercises multipart disk spooling.
    metadata = b'Comment\0' + b'x'*(1024*1024+1)
    ancillary = struct.pack('>I', len(metadata)) + b'tEXt' + metadata + struct.pack('>I', zlib.crc32(b'tEXt'+metadata))
    large_source = source[:33] + ancillary + source[33:]
    large_id = upload(owner, large_source, {'name': 'negative'})
    wait_task(owner, large_id)
    large_result, _ = expect('GET', '/result/'+large_id, 200, token=owner)
    assert decode_png(large_result)[2] == [(255-r, 255-g, 255-b, a) for r,g,b,a in pixels]

    upload(owner, b'not an image', {'name': 'negative'}, 400)
    upload(owner, source, {'name': 'unsupported'}, 400)
    upload(owner, source, {'name': 'blur'}, 400)
    upload(owner, source, {'name': 'sharpen', 'parameters': {'sigma': 11}}, 400)
    upload(owner, source, {'name': 'negative', 'parameters': {'sigma': 1}}, 400)
    upload(owner, b'x'*(10*1024*1024+1), {'name': 'negative'}, 413)
    # PNG has a valid header but no pixel data: decode fails in the processor.
    failed_id = upload(owner, source[:33], {'name': 'negative'})
    wait_task(owner, failed_id, 'failed')
    expect('GET', '/result/'+failed_id, 422, token=owner)
    # A failed job must not block the next Kafka message.
    next_id = upload(owner, source, {'name': 'negative'})
    wait_task(owner, next_id)

    secret = os.getenv('INTERNAL_TOKEN')
    if not secret and (ROOT / '.env').exists():
        secret = next(line.split('=', 1)[1] for line in (ROOT / '.env').read_text().splitlines() if line.startswith('INTERNAL_TOKEN='))
    if secret:
        expect('POST', '/commit', 204, body={'task_id': next_id}, token=secret)
        expect('POST', '/commit', 204, body={'task_id': next_id, 'error': 'duplicate failure'}, token=secret)
        data, _ = expect('GET', '/status/'+next_id, 200, token=owner)
        assert data['status'] == 'ready'
        expect('POST', '/commit', 404, body={'task_id': str(uuid.uuid4())}, token=secret)

    if args.restart_processor:
        subprocess.run(['docker', 'compose', 'stop', 'processor'], cwd=ROOT, check=True, stdout=subprocess.DEVNULL)
        try:
            pending = upload(owner, source, {'name': 'negative'})
            data, _ = expect('GET', '/status/'+pending, 200, token=owner)
            assert data['status'] == 'in_progress'
            expect('GET', '/result/'+pending, 202, token=owner)
        finally:
            subprocess.run(['docker', 'compose', 'start', 'processor'], cwd=ROOT, check=True, stdout=subprocess.DEVNULL)
        wait_task(owner, pending)
        expect('GET', '/result/'+pending, 200, token=owner)

    print(f'PASS: {checks} HTTP checks; filter pixels, ownership, failure handling and callback idempotency verified.')

if __name__ == '__main__':
    main()

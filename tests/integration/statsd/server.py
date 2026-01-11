import socket
import sys

addr = ("0.0.0.0", 8125)
log_path = "/data/metrics.log"

sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
sock.bind(addr)

with open(log_path, "a", buffering=1) as f:
    while True:
        data, _ = sock.recvfrom(65535)
        if not data:
            continue
        try:
            f.write(data.decode("utf-8", errors="replace").strip() + "\n")
        except Exception as exc:
            print(exc, file=sys.stderr)

#!/usr/bin/env python3
"""Start two local processes, curl a public HTTPS website, always clean up.

Usage: python3 scripts/smoke.py /path/to/tunnel /path/to/test-credentials
Generates no cloud resources. Downloads only the public example.com page.
"""
import pathlib
import socket
import subprocess
import sys
import time


def port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def ready(proc, number):
    deadline = time.monotonic() + 5
    while time.monotonic() < deadline:
        if proc.poll() is not None:
            raise RuntimeError("process exited before listening")
        try:
            with socket.create_connection(("127.0.0.1", number), timeout=0.1):
                return
        except OSError:
            time.sleep(0.05)
    raise TimeoutError("local listener did not start")


def main():
    binary = str(pathlib.Path(sys.argv[1]).resolve())
    certs = pathlib.Path(sys.argv[2]).resolve()
    gate_port, voyager_port = port(), port()
    processes = []
    with (certs / "smoke-processes.log").open("w") as log:
        try:
            b = subprocess.Popen([
                binary, "-mode", "voyager", "-listen", f"127.0.0.1:{voyager_port}",
                "-token-file", str(certs / "token.txt"),
                "-inner-cert", str(certs / "inner.pem"),
                "-inner-key", str(certs / "inner.key"),
                "-outer-cert", str(certs / "outer.pem"),
                "-outer-key", str(certs / "outer.key"),
            ], stdout=log, stderr=log)
            processes.append(b)
            ready(b, voyager_port)
            a = subprocess.Popen([
                binary, "-mode", "local", "-listen", f"127.0.0.1:{gate_port}",
                "-token-file", str(certs / "token.txt"),
                "-inner-ca", str(certs / "inner-ca.pem"),
                "-outer-ca", str(certs / "outer-ca.pem"),
                "-voyager-url", f"wss://localhost:{voyager_port}/tunnel",
            ], stdout=log, stderr=log)
            processes.append(a)
            ready(a, gate_port)
            result = subprocess.run([
                "curl", "--silent", "--show-error", "--fail", "--max-time", "30",
                "--noproxy", "", "--proxy", f"http://127.0.0.1:{gate_port}",
                "https://example.com/",
            ], capture_output=True, text=True, timeout=35)
            if result.returncode:
                raise RuntimeError(f"curl failed ({result.returncode}): {result.stderr.strip()}")
            if "Example Domain" not in result.stdout:
                raise AssertionError("unexpected website content")
            print(f"PASS: curl -> Gate -> WSS + inner TLS -> Voyager -> https://example.com/ ({len(result.stdout.encode())} bytes)")
        finally:
            for proc in reversed(processes):
                proc.terminate()
            for proc in processes:
                try:
                    proc.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    proc.kill()
                    proc.wait()


if __name__ == "__main__":
    main()

import socket,ssl
try:
    s=socket.create_connection(("api.ipify.org",443),timeout=6)
    c=ssl.create_default_context().wrap_socket(s,server_hostname="api.ipify.org")
    c.sendall(b"GET /?format=json HTTP/1.1\r\nHost: api.ipify.org\r\nConnection: close\r\n\r\n")
    d=b""
    while True:
        x=c.recv(4096)
        if not x: break
        d+=x
    print(d.decode("utf-8","replace").split("\r\n\r\n")[-1])
except Exception as e:
    print("ERR", e)
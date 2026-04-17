from http.server import HTTPServer, BaseHTTPRequestHandler

BXML = """<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <SpeakSentence voice="julie">Hello! This is a test call from Bandwidth. Have a great day. Goodbye!</SpeakSentence>
    <Hangup/>
</Response>"""

class CallbackHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Type", "application/xml")
        self.end_headers()
        self.wfile.write(BXML.encode())

    def do_POST(self):
        content_length = int(self.headers.get("Content-Length", 0))
        self.rfile.read(content_length)
        self.send_response(200)
        self.send_header("Content-Type", "application/xml")
        self.end_headers()
        self.wfile.write(BXML.encode())

print("Callback server running on http://localhost:80")
HTTPServer(("0.0.0.0", 80), CallbackHandler).serve_forever()

"""Direct, redirect-refusing HTTP for acceptance credentials and private bodies.

Keep the caller's ordinary TLS verification and HTTPError handling. Do not use
ambient HTTP proxies or let a redirect choose another credential destination.
"""
import urllib.request


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, file_pointer, code, message, headers, new_url):
        return None


def private_opener():
    return urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())

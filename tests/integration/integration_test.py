import os
import pytest
import urllib.parse

import requests

import urllib3.util

URL_HOST=os.environ.get("URL_HOST", "localhost")

@pytest.mark.skipif(os.environ.get("SKIP_KUBERNETES_INTEGRATION_TESTS"), "SKIP_KUBERNETES_INTEGRATION_TESTS set in environment")
def test_nonexistent_route():
    # no matching ingress rule, so expect a 404
    headers = {
        "Host": "chilledornaments.com",
    }

    r = requests.get(f"http://{URL_HOST}/zzzzzz", headers=headers, timeout=2, allow_redirects=False)

    assert r.status_code == 404
    assert not r.headers.get("Cache-Control")

def test_nonexistent_route_with_valid_ingress():
    # matching ingress rule, no matching redirector rule
    headers = {
        "Host": "routeme.com",
    }

    r = requests.get(f"http://{URL_HOST}/zzzzzz", headers=headers, timeout=2, allow_redirects=False)

    assert r.status_code == 404
    assert not r.headers.get("Cache-Control")
    assert not r.headers.get("Location")

def test_route_without_regex():
    headers = {
        "Host": "example.com",
    }

    r = requests.get(f"http://{URL_HOST}/test/foo", headers=headers, timeout=2,  allow_redirects=False)
    assert r.headers.get("Location") == "https://foo.com/destination/d"
    assert r.status_code == 301, r.status_code

def test_blog_rule():
    headers = {
        "Host": "localhost",
    }

    r = requests.get(f"http://{URL_HOST}/blog/2024/01/01/foo/bar", headers=headers, timeout=2, allow_redirects=False)
    assert r.headers.get("Location") == "https://blog.localhost.com/posts/foo/bar"
    assert r.status_code == 301, r.status_code

def test_combine_params():
    headers = {
        "Host": "localhost",
    }

    params = {
        "should": "stay",
        "new": "goodbye"
    }

    expected_params = {
        "should": ["stay"],
        "new": ["hello"],
        "existing": ["world"],
    }

    r = requests.get(f"http://{URL_HOST}/params/test", headers=headers, timeout=2,  allow_redirects=False, params=params)

    assert r.status_code == 301, r.status_code


    u = urllib3.util.parse_url(r.headers.get("Location"))
    q = urllib.parse.parse_qs(u.query)
    for k, v in expected_params.items():
        assert q.get(k) == v

def test_no_rule_level_cache_control():
    headers = {
        "Host": "localhost",
    }

    params = {
        "should": "stay",
        "new": "goodbye"
    }

    r = requests.get(f"http://{URL_HOST}/params/test", headers=headers, timeout=2,  allow_redirects=False, params=params)

    assert r.status_code == 301, r.status_code
    assert r.headers.get("Cache-Control") == "max-age=60"

def test_rule_level_cache_control():
    headers = {
        "Host": "localhost",
    }

    r = requests.get(f"http://{URL_HOST}/blog/2024/01/01/foo/bar", headers=headers, timeout=2, allow_redirects=False)
    assert r.headers.get("Location") == "https://blog.localhost.com/posts/foo/bar"
    assert r.status_code == 301, r.status_code
    assert r.headers.get("Cache-Control") == "max-age=3", r.headers.items()


def test_rule_cache_control_disabled():
    headers = {
        "Host": "localhost",
    }

    r = requests.get(f"http://{URL_HOST}/params/test2", headers=headers, timeout=2, allow_redirects=False)
    assert r.headers.get("Location") == "https://demo.localhost.com/?new=hello"
    assert r.status_code == 301, r.status_code
    assert not r.headers.get("Cache-Control"), r.headers.items()

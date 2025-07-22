#!/usr/bin/env python3
# /// script
# dependencies = [
#   "requests",
# ]
# ///

import requests

response = requests.get("https://api.github.com")
print(f"GitHub API Status: {response.status_code}")
print(f"Rate Limit: {response.headers.get('X-RateLimit-Remaining', 'N/A')}")
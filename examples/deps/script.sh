#!/usr/bin/env apko-shell
#!apko-shell -p curl,jq

echo "Fetching GitHub API info for Chainguard..."
echo

# Fetch and parse JSON from GitHub API
curl -s https://api.github.com/orgs/chainguard-dev | jq '{
  name: .name,
  description: .description,
  public_repos: .public_repos,
  created: .created_at,
  blog: .blog
}'

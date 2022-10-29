# Testing with Smee

* https://smee.io/
* https://www.jenkins.io/blog/2019/01/07/webhook-firewalls/
* https://github.com/probot/smee-client

## Local Setup

### Install Smee Client

```shell
npm install -g smee-client
```

### Run Smee Client

```shell
smee --port 1323 --path /v1/github/
```

### Update Webhook

* https://cli.github.com/manual/gh_api

```shell
WEBHOOK_RAW=""
export WEBHOOK_URL=$(echo $WEBHOOK_RAW | python3 -c "import urllib.parse, sys; print(urllib.parse.quote(sys.stdin.read()))")
echo "WEBHOOK_URL=$WEBHOOK_URL"
```

```shell
curl \
  -X PATCH \
  -H "Accept: application/vnd.github+json" \
  -H "Authorization: Bearer ${GH_API_TOKEN}" \
  "https://api.github.com/repos/joostvdg/gitstafette/hooks/${WEBHOOK_ID}" \
  -d '{"config": {"url": "${WEBHOOK_URL}"}}'
```

This doesn't work:

```shell
gh api -H "Accept: application/vnd.github+json"\
  https://api.github.com/repos/joostvdg/gitstafette/hooks/${WEBHOOK_ID} \
  --method PATCH \
  --field "url=https://joostvdg.github.io"
```


```shell
gh: Not Found (HTTP 404)
{
"message": "Not Found",
"documentation_url": "https://docs.github.com/rest/reference/repos#update-a-repository-webhook"
}
gh: This API operation needs the "admin:repo_hook" scope. To request it, run:  gh auth refresh -h github.com -s admin:repo_hook
```

### List Webhook

```shell
gh api -H "Accept: application/vnd.github+json" /repos/joostvdg/gitstafette/hooks
```

```shell
WEBHOOK_ID=$(gh api -H "Accept: application/vnd.github+json" /repos/joostvdg/gitstafette/hooks | jq '.[].id')
```

## Get Repository ID

```shell
gh api -H "Accept: application/vnd.github+json" /repos/joostvdg/gitstafette
```

```shell
REPO_ID=$(gh api -H "Accept: application/vnd.github+json" /repos/joostvdg/gitstafette | jq '.id')
echo REPO_ID=${REPO_ID}
```
cachebot
========

A [Slack](https://slack.com/) bot for [CloudFlare](https://www.cloudflare.com/).

![Screenshot of cachebot](screenshot.png)

### Configuration

Configure cachebot using environment variables:

| Environment variable | Description
| -------------------- | -----------
| CF_TOKEN             | CloudFlare API token
| CF_EMAIL             | CloudFlare account e-mail
| CF_ZONE              | CloudFlare Zone ID
| SLACK_TOKEN          | Slack API token
| SLACK_CHANNEL        | Slack channel (including `#`)
| URL_BASES            | Base URL(s), comma separated
| URL_SUFFIXES         | URL suffixes, comma separated

### Usage

1. Start cachebot
2. `/invite` cachebot to a channel
3. Ask cachebot to clear your cache:
  - `clear cache`
  - `clear cache for /some/uri`

### License

Copyright ©‎ 2016, Ian Kent (http://iankent.uk).

Released under MIT license, see [LICENSE](LICENSE.md) for details.

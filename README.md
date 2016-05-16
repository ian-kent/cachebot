cachebot
========

A [Slack](https://slack.com/) bot for [CloudFlare](https://www.cloudflare.com/).

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

### License

Copyright ©‎ 2016, Ian Kent (http://iankent.uk).

Released under MIT license, see [LICENSE](LICENSE.md) for details.

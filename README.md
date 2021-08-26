# @recally_bot - Telegram bot to boost your memory

This repo holds the sources of [@recally_bot](https://t.me/recally_bot) — telegram bot that helps you to memorize any stuff using interval memorization technic. Just send it any text and it will send it back to you in certain time intervals (1, 5, 20 minutes, then 1, 8, 24, 48 hours).

## Project setup

### Production

Run in console of your AWS or something:

`TIMER_BOT_TOKEN=<YOUR_BOT_TOKEN> TIMER_DB_HOST=<YOUR_BOT_POSTGRES_DB_HOST> TIMER_DB_PASSWORD=<YOUR_DB_PASSWORD> nohup go run main.go &`
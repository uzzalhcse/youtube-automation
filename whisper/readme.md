# Option 1: Run Both Profiles Together
## Download models and start API in one command
`docker compose --profile download --profile api up --build`
# Option 2: Run Sequentially (Recommended)
## First, download the models
`docker compose --profile download up model-downloader`

## Then start the API (after models are downloaded)
`docker compose --profile api up --build whisper-api`
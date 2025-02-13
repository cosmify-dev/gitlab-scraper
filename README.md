# GitLab Scraper

## Setup

1. Copy `.envrc.tpl` to `.envrc` and fill in the required values.
2. Run the following command to start the services:

    ```sh
    docker compose -f ./infra/docker-compose.yml up -d
    ```

3. Run the GitLab scrapper:

    ```sh
    go run main.go scrape --config path/to/your/config.json
    ```

4. Access Prometheus at [http://localhost:9090](http://localhost:9090).
5. Access Push Gateway at [http://localhost:9091](http://localhost:9091).

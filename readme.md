# [GTG] Live Map

A real-time, feature-rich tactical live map for Arma Reforger servers. Built with a Go backend and a modern JavaScript frontend, designed for scalability and performance.

[![Discord](https://img.shields.io/discord/YOUR_DISCORD_ID?label=Join%20Discord&logo=discord&color=7289DA)](https://discord.com/invite/gtg-german-tactical-group)

---

## Features

-   **Live Tactical Map:** Real-time tracking of player and vehicle positions.
-   **Powerful Replay System:** A full timeline with play/pause, a time-range selector, and event markers to analyze past operations.
-   **Heatmap Analysis:** Visualize hotspots and player movement patterns for any given time period.
-   **User & Server Management:** A full dashboard for server owners to add, manage, and configure their servers.
-   **Discord Integration:** Secure and simple login using Discord OAuth2.
-   **Advanced Permissions:**
    -   Admin roles with full access.
    -   Grant access to your private maps to other users.
    -   Keep maps private, public-by-link, or list them publicly.
-   **Multiple Data Sources:**
    -   **Push Method:** In-game mod sends position data directly to the API.
    -   **Pull Method:** A scalable background worker fetches damage/kill logs automatically via FTP, FTPS, or SFTP (with password or SSH key auth).
-   **Powerful Server Management API:** Automate server administration tasks like changing maps or managing user access directly via the API, perfect for integration with game server panels or Discord bots.
-   **Built for Scale:** Decoupled architecture using Redis for job queuing and session management allows for horizontal scaling of web and worker instances.
-   **Secure by Design:** Rate limiting, hashed API keys, encrypted credentials, and server-side validation protect the platform.

## Getting Started

### Prerequisites

-   [Docker](https://www.docker.com/get-started)
-   [Docker Compose](https://docs.docker.com/compose/install/)

### Installation & Setup

#### 1. Clone the Repository
Clone this repository to your local machine or server.
```bash
git clone <your-repository-url>
cd gtg-livemap
```

#### 2. Configure the Backend
You need to edit two configuration files before the first start.

**A) Docker Compose (`docker-compose.yaml`)**
This file orchestrates the services. You **must** set secure passwords here.
-   Open `docker-compose.yaml`.
-   Change the `MYSQL_ROOT_PASSWORD` to a strong, unique password.
-   Change the `MYSQL_USER` and `MYSQL_PASSWORD` if you wish.

**B) Application Configuration (`config.yaml`)**
This file contains all the secrets for the application itself.
-   Open `config.yaml`.
-   Make sure the database `user`, `password`, and `dbname` match the `MYSQL_...` variables you set in `docker-compose.yaml`.
-   **Crucially**, set your Discord Application secrets under the `discord` section.
-   Change the `session.secret` and `encryption.aes_key` to long, random, and unique strings.

#### 3. Build and Run with Docker Compose
Once configured, you can build and start the entire stack with a single command:
```bash
docker-compose up --build -d
```
-   `--build`: Builds the Go application Docker image. Only needed on first run or after changing Go code.
-   `-d`: Runs the containers in the background.

The application will be available at `http://localhost:8080`.

## Game Server Configuration

To feed data into the Live Map, you need to configure your Arma Reforger game server with specific mods.

### 1. Position Data (API Push Method) - Required
This method requires the game server to send position data to our API.

-   **Required Mod:** [GTG LiveMap & Positions Logger](https://reforger.armaplatform.com/workshop/65E90CB6053F2791)
-   **Setup:**
    1.  Add the mod to your server.
    2.  In your server's user profile folder, create a new config file here: `<PROFILEFOLDER>/profile/GTG_LiveMap_config.json`
    3.  Add the following content to the JSON file:

    ```json
    {
        "contextURL": "http://YOUR_SERVER_IP:8080/api/v1/positions",
        "APIToken": "YOUR_API_KEY_FROM_DASHBOARD",
        "intervalLog": 1,
        "intervalAPI": 1
    }
    ```
    -   Replace `http://YOUR_SERVER_IP:8080` with the public URL or IP of your GTG Live Map instance.
    -   Replace `YOUR_API_KEY_FROM_DASHBOARD` with the unique API key you generated in the dashboard.

### 2. Damage & Kill Logs (SFTP/FTP Pull Method) - Optional
This method allows the GTG Live Map backend to connect to your server and fetch log files automatically.

-   **Required Mod:** [ReforgerJSLogging](https://reforger.armaplatform.com/workshop/6572332E971992EF-ReforgerJSLogging)
-   **Setup:**
    1.  Add the mod to your server. It will generate logs in `<PROFILEFOLDER>/profile/RJS/events/`.
    2.  Create a **read-only** FTP or SFTP user on your game server machine.
    3.  In the GTG Live Map Dashboard, use the **Setup Wizard** or go to `Manage Server` -> `Log Source` tab to enter your connection details.

---

## Server Management API Documentation
These endpoints allow for automated management of a server using its API Key. All requests must include the API Key, either in the `X-API-KEY` header or as a `Bearer` token in the `Authorization` header.

### Access Management

-   **`GET /api/v1/server/access`**
    -   **Description:** Lists the Discord User IDs of all users who have been granted access to the server.
    -   **Success Response (200 OK):** `[ "123456789...", "987654321..." ]`

-   **`POST /api/v1/server/access`**
    -   **Description:** Grants a user access to the server. The user does **not** need to exist in the `users` table.
    -   **Body:** `{ "user_id": "123456789..." }`
    -   **Success Response (201 Created):** `{ "message": "Access granted successfully" }`

-   **`DELETE /api/v1/server/access/:user_id`**
    -   **Description:** Revokes access for a specific Discord User ID.
    -   **Success Response (200 OK):** `{ "message": "Access revoked successfully" }`

### Map Management

-   **`GET /api/v1/public/map-configs`**
    -   **Description:** (Public Endpoint) Lists all available map configurations and their IDs.
    -   **Success Response (200 OK):** A JSON array of map config objects.

-   **`GET /api/v1/server/map`**
    -   **Description:** Gets the current map configuration for the server.
    -   **Success Response (200 OK):** A single map config JSON object.

-   **`PUT /api/v1/server/map`**
    -   **Description:** Sets a new map for the server.
    -   **Body:** `{ "map_id": 5 }`
    -   **Success Response (200 OK):** `{ "message": "Server map updated successfully" }`



## Using the Application

### The Live Map
The core of the application. View real-time movements, use the timeline slider to replay events, analyze specific periods, and generate heatmaps.

### The Dashboard
Your personal management area after logging in. Add new servers, manage existing ones (name, map, visibility), configure the FTP/SFTP log fetcher, and grant or revoke access for other users.

---

## Advanced Docker Usage

### Changing the Run Mode
Edit the `APP_MODE` environment variable in `docker-compose.yaml` for scalability:
-   `APP_MODE=all`: (Default) Web server, scheduler, and consumer in one container.
-   `APP_MODE=web`: Only the web-facing API and frontend.
-   `APP_MODE=scheduler`: Only the job scheduler (run only one instance).
-   `APP_MODE=consumer`: Only a background job worker (can be scaled).

### Seeding the Bad Words Database
To populate the database with a list of inappropriate words for name filtering, run this one-off command:
```bash
docker-compose run --rm app /gtg-live-map-server -mode seed-bad-words
```
This starts a temporary container, runs the script, and removes itself.

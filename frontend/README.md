# Advanced Blockchain Go - Frontend Visualization

This is the frontend visualization for the Advanced Blockchain Go project. It provides a user-friendly interface to monitor and interact with the blockchain system.

## Features

- **Dashboard:** Overview of blockchain metrics, charts, and recent activity
- **Blockchain Viewer:** Visual representation of the blockchain with blocks and connections
- **Shard Details:** Detailed information about each shard
- **Block Explorer:** Examine the contents of individual blocks
- **Transaction Pool:** View pending transactions
- **Network Stats:** Monitor network performance and node status

## Prerequisites

- Node.js (v14 or newer)
- NPM or Yarn
- Go backend running on http://localhost:8080

## Getting Started

1. Install dependencies:

```bash
npm install
```

2. Start the development server:

```bash
npm start
```

3. Open [http://localhost:3000](http://localhost:3000) to view the application in your browser.

## Tech Stack

- React with TypeScript
- React Router for navigation
- Bootstrap for UI components
- Chart.js for data visualization
- D3.js for interactive blockchain visualization
- Axios for API requests

## Connecting to the Backend

The frontend expects the Go blockchain backend to be running on `http://localhost:8080`. The API service in `src/services/api.ts` is configured to connect to this endpoint. If your backend is running on a different URL, update the `API_URL` constant in that file.

## Available Scripts

- `npm start`: Run the app in development mode
- `npm test`: Launch the test runner
- `npm run build`: Build the app for production
- `npm run eject`: Eject from Create React App

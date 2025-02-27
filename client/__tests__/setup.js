const { spawn } = require('child_process');
const waitOn = require('wait-on');

module.exports = async () => {
    // Start the server if it's not already running
    const server = spawn('go', ['run', '../main.go']);

    // Wait for the server to be ready
    await waitOn({
        resources: ['http://localhost:3000/ws'],
        timeout: 30000,
    });

    // Save the server process to shut it down after tests
    global.__SERVER__ = server;
}; 
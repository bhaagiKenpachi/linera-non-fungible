const WebSocket = require('ws');

describe('WebSocket Ping/Pong Tests', () => {
    let ws;
    const WS_URL = 'ws://localhost:3000/ws';

    beforeEach((done) => {
        ws = new WebSocket(WS_URL);
        
        ws.onopen = () => {
            done();
        };

        ws.onerror = (error) => {
            done(error);
        };
    });

    afterEach((done) => {
        if (ws.readyState === WebSocket.OPEN) {
            ws.close();
        }
        done();
    });

    test('should receive connection message on connect', (done) => {
        ws.onmessage = (event) => {
            const message = JSON.parse(event.data);
            expect(message).toEqual({
                type: 'connected',
                data: 'Successfully connected to WebSocket'
            });
            done();
        };
    });

    test('should receive pong response to ping message', (done) => {
        // Wait for initial connection message
        ws.onmessage = () => {
            // After connection message, send ping
            const pingMessage = {
                type: 'ping',
                data: 'ping'
            };
            
            // Change message handler to check pong response
            ws.onmessage = (event) => {
                const message = JSON.parse(event.data);
                expect(message).toEqual({
                    type: 'pong',
                    data: 'pong'
                });
                done();
            };

            ws.send(JSON.stringify(pingMessage));
        };
    });

    test('should receive error for unknown message type', (done) => {
        // Wait for initial connection message
        ws.onmessage = () => {
            // After connection message, send unknown type
            const unknownMessage = {
                type: 'unknown',
                data: 'test'
            };
            
            // Change message handler to check error response
            ws.onmessage = (event) => {
                const message = JSON.parse(event.data);
                expect(message).toEqual({
                    type: 'error',
                    error: 'Unknown message type'
                });
                done();
            };

            ws.send(JSON.stringify(unknownMessage));
        };
    });

    test('should handle multiple ping/pong exchanges', (done) => {
        let pongCount = 0;
        const expectedPongs = 3;

        // Wait for initial connection message
        ws.onmessage = () => {
            // After connection, set up message handler for pongs
            ws.onmessage = (event) => {
                const message = JSON.parse(event.data);
                if (message.type === 'pong') {
                    pongCount++;
                    expect(message).toEqual({
                        type: 'pong',
                        data: 'pong'
                    });

                    if (pongCount === expectedPongs) {
                        done();
                    } else {
                        // Send another ping
                        ws.send(JSON.stringify({
                            type: 'ping',
                            data: 'ping'
                        }));
                    }
                }
            };

            // Send first ping
            ws.send(JSON.stringify({
                type: 'ping',
                data: 'ping'
            }));
        };
    });

    test('should handle connection timeout', (done) => {
        const ws2 = new WebSocket('ws://invalid-url:12345');
        
        ws2.onerror = () => {
            expect(ws2.readyState).toBe(WebSocket.CLOSED);
            done();
        };
    });
}); 
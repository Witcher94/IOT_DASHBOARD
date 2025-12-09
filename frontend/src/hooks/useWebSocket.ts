import { useEffect, useRef, useCallback } from 'react';
import { useAuthStore } from '../contexts/authStore';
import type { WebSocketMessage } from '../types';

type MessageHandler = (message: WebSocketMessage) => void;

export function useWebSocket(onMessage: MessageHandler) {
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const { token, isAuthenticated } = useAuthStore();

  const connect = useCallback(() => {
    if (!isAuthenticated || !token) return;

    const wsUrl = `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/api/v1/ws`;
    
    try {
      const ws = new WebSocket(wsUrl);
      
      ws.onopen = () => {
        console.log('WebSocket connected');
        // Send auth token
        ws.send(JSON.stringify({ type: 'auth', token }));
      };

      ws.onmessage = (event) => {
        try {
          const message = JSON.parse(event.data) as WebSocketMessage;
          onMessage(message);
        } catch (error) {
          console.error('Failed to parse WebSocket message:', error);
        }
      };

      ws.onclose = () => {
        console.log('WebSocket disconnected');
        // Reconnect after 5 seconds
        reconnectTimeoutRef.current = setTimeout(connect, 5000);
      };

      ws.onerror = (error) => {
        console.error('WebSocket error:', error);
        ws.close();
      };

      wsRef.current = ws;
    } catch (error) {
      console.error('Failed to connect WebSocket:', error);
      reconnectTimeoutRef.current = setTimeout(connect, 5000);
    }
  }, [isAuthenticated, token, onMessage]);

  useEffect(() => {
    connect();

    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      if (wsRef.current) {
        wsRef.current.close();
      }
    };
  }, [connect]);

  const send = useCallback((data: unknown) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(data));
    }
  }, []);

  return { send };
}


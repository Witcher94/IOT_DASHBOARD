import { useEffect, useRef, useCallback } from 'react';
import { useAuthStore } from '../contexts/authStore';
import type { WebSocketMessage } from '../types';

type MessageHandler = (message: WebSocketMessage) => void;

// API base URL
const API_URL = import.meta.env.VITE_API_URL || `${window.location.origin}/api/v1`;

export function useWebSocket(onMessage: MessageHandler) {
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const { token, isAuthenticated } = useAuthStore();

  const connect = useCallback(async () => {
    if (!isAuthenticated || !token) return;

    try {
      // Step 1: Get one-time ticket (with JWT auth)
      const ticketResponse = await fetch(`${API_URL}/ws/ticket`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json',
        },
      });

      if (!ticketResponse.ok) {
        console.error('Failed to get WebSocket ticket');
        reconnectTimeoutRef.current = setTimeout(connect, 5000);
        return;
      }

      const { ticket } = await ticketResponse.json();

      // Step 2: Connect WebSocket with ticket
      const wsUrl = `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/api/v1/ws?ticket=${ticket}`;
      
      const ws = new WebSocket(wsUrl);
      
      ws.onopen = () => {
        console.log('WebSocket connected');
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

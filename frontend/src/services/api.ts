import axios, { AxiosError } from 'axios';
import { useAuthStore } from '../contexts/authStore';
import type {
  User,
  Device,
  DeviceShare,
  Metric,
  Command,
  DashboardStats,
  CreateDeviceRequest,
  CreateCommandRequest,
  GatewayTopology,
  Card,
  AccessLog,
  UpdateCardStatusRequest,
} from '../types';

// В production API на тому ж домені через LB path routing
const API_URL = import.meta.env.VITE_API_URL || `${window.location.origin}/api/v1`;

const api = axios.create({
  baseURL: API_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Request interceptor to add auth token
api.interceptors.request.use((config) => {
  const token = useAuthStore.getState().token;
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// Response interceptor for error handling
api.interceptors.response.use(
  (response) => response,
  (error: AxiosError) => {
    if (error.response?.status === 401) {
      useAuthStore.getState().logout();
      window.location.href = '/login';
    }
    return Promise.reject(error);
  }
);

// Auth
export const authApi = {
  getGoogleLoginUrl: () => `${API_URL}/auth/google`,
  
  getCurrentUser: async (): Promise<User> => {
    const { data } = await api.get('/me');
    return data;
  },
  
  refreshToken: async (token: string): Promise<{ token: string }> => {
    const { data } = await api.post('/auth/refresh', { token });
    return data;
  },
};

// Devices
export const devicesApi = {
  getAll: async (): Promise<Device[]> => {
    const { data } = await api.get('/devices');
    return data || [];
  },
  
  getById: async (id: string): Promise<Device> => {
    const { data } = await api.get(`/devices/${id}`);
    return data;
  },
  
  create: async (req: CreateDeviceRequest): Promise<Device> => {
    const { data } = await api.post('/devices', req);
    return data;
  },
  
  delete: async (id: string): Promise<void> => {
    await api.delete(`/devices/${id}`);
  },
  
  regenerateToken: async (id: string): Promise<{ token: string }> => {
    const { data } = await api.post(`/devices/${id}/regenerate-token`);
    return data;
  },
  
  updateAlertSettings: async (id: string, settings: {
    alerts_enabled?: boolean;
    alert_temp_min?: number;
    alert_temp_max?: number;
    alert_humidity_max?: number;
  }): Promise<Device> => {
    const { data } = await api.put(`/devices/${id}/alerts`, settings);
    return data;
  },

  // Sharing
  shareDevice: async (id: string, email: string, permission: string = 'view'): Promise<DeviceShare> => {
    const { data } = await api.post(`/devices/${id}/shares`, { email, permission });
    return data;
  },

  getShares: async (id: string): Promise<DeviceShare[]> => {
    const { data } = await api.get(`/devices/${id}/shares`);
    return data || [];
  },

  removeShare: async (id: string, userId: string): Promise<void> => {
    await api.delete(`/devices/${id}/shares/${userId}`);
  },

  getSharedWithMe: async (): Promise<Device[]> => {
    const { data } = await api.get('/shared-devices');
    return data || [];
  },
};

// Metrics
export const metricsApi = {
  getByDeviceId: async (deviceId: string, limit?: number): Promise<Metric[]> => {
    const params = limit ? { limit } : {};
    const { data } = await api.get(`/devices/${deviceId}/metrics`, { params });
    return data || [];
  },
  
  getByPeriod: async (deviceId: string, period: string): Promise<Metric[]> => {
    const { data } = await api.get(`/devices/${deviceId}/metrics`, {
      params: { period },
    });
    return data || [];
  },
};

// Commands
export const commandsApi = {
  getByDeviceId: async (deviceId: string): Promise<Command[]> => {
    const { data } = await api.get(`/devices/${deviceId}/commands`);
    return data || [];
  },
  
  create: async (deviceId: string, req: CreateCommandRequest): Promise<Command> => {
    const { data } = await api.post(`/devices/${deviceId}/commands`, req);
    return data;
  },
  
  cancel: async (deviceId: string, commandId: string): Promise<void> => {
    await api.delete(`/devices/${deviceId}/commands/${commandId}`);
  },
};

// Dashboard
export const dashboardApi = {
  getStats: async (): Promise<DashboardStats> => {
    const { data } = await api.get('/dashboard/stats');
    return data;
  },
};

// Admin
export const adminApi = {
  getAllUsers: async (): Promise<User[]> => {
    const { data } = await api.get('/admin/users');
    return data || [];
  },
  
  getAllDevices: async (): Promise<Device[]> => {
    const { data } = await api.get('/admin/devices');
    return data || [];
  },
  
  deleteUser: async (id: string): Promise<void> => {
    await api.delete(`/admin/users/${id}`);
  },
  
  updateUserRole: async (id: string, isAdmin: boolean): Promise<User> => {
    const { data } = await api.put(`/admin/users/${id}/role`, { is_admin: isAdmin });
    return data;
  },
  
  getUserDevices: async (userId: string): Promise<Device[]> => {
    const { data } = await api.get(`/admin/users/${userId}/devices`);
    return data || [];
  },
};

// Gateway
export const gatewayApi = {
  getTopology: async (gatewayId: string): Promise<GatewayTopology> => {
    const { data } = await api.get(`/gateways/${gatewayId}/topology`);
    return data;
  },
  
  sendCommandToNode: async (gatewayId: string, nodeId: string, command: CreateCommandRequest): Promise<Command> => {
    const { data } = await api.post(`/gateways/${gatewayId}/nodes/${nodeId}/commands`, command);
    return data;
  },

  sendCommand: async (deviceId: string, command: CreateCommandRequest): Promise<Command> => {
    const { data } = await api.post(`/devices/${deviceId}/commands`, command);
    return data;
  },

  getLogs: async (gatewayId: string, type: 'serial' | 'gateway'): Promise<LogEntry[]> => {
    const { data } = await api.get(`/gateways/${gatewayId}/logs/${type}`);
    return data;
  },
};

export interface LogEntry {
  timestamp: string;
  level: string;
  message: string;
}

// SKUD (Access Control)
// Note: Access devices are now created via the regular Devices page.
// SKUD uses the same X-Device-Token as IoT devices.
export const skudApi = {
  // Cards - filter by device_id to get cards linked to a specific device
  getCards: async (status?: string, device_id?: string): Promise<Card[]> => {
    const params: Record<string, string> = {};
    if (status) params.status = status;
    if (device_id) params.device_id = device_id;
    const { data } = await api.get('/skud/cards', { params });
    return data || [];
  },

  getCard: async (id: string): Promise<Card> => {
    const { data } = await api.get(`/skud/cards/${id}`);
    return data;
  },

  updateCardStatus: async (id: string, req: UpdateCardStatusRequest): Promise<Card> => {
    const { data } = await api.patch(`/skud/cards/${id}/status`, req);
    return data;
  },

  deleteCard: async (id: string): Promise<void> => {
    await api.delete(`/skud/cards/${id}`);
  },

  // Card-Device links
  linkCardToDevice: async (cardId: string, deviceId: string): Promise<Card> => {
    const { data } = await api.post(`/skud/cards/${cardId}/devices/${deviceId}`);
    return data;
  },

  unlinkCardFromDevice: async (cardId: string, deviceId: string): Promise<Card> => {
    const { data } = await api.delete(`/skud/cards/${cardId}/devices/${deviceId}`);
    return data;
  },

  // Access Logs
  getAccessLogs: async (filters?: {
    action?: string;
    allowed?: boolean;
    card_uid?: string;
    device_id?: string;
    limit?: number;
  }): Promise<AccessLog[]> => {
    const params: Record<string, string> = {};
    if (filters?.action) params.action = filters.action;
    if (filters?.allowed !== undefined) params.allowed = String(filters.allowed);
    if (filters?.card_uid) params.card_uid = filters.card_uid;
    if (filters?.device_id) params.device_id = filters.device_id;
    if (filters?.limit) params.limit = String(filters.limit);
    
    const { data } = await api.get('/skud/logs', { params });
    return data || [];
  },
};

export default api;


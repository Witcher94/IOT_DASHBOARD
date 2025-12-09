import axios, { AxiosError } from 'axios';
import { useAuthStore } from '../contexts/authStore';
import type {
  User,
  Device,
  Metric,
  Command,
  DashboardStats,
  CreateDeviceRequest,
  CreateCommandRequest,
} from '../types';

const API_URL = import.meta.env.VITE_API_URL || '/api/v1';

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
    const { data } = await api.get('/devices', { params: { all: true } });
    return data || [];
  },
};

export default api;


import { useCallback, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { motion } from 'framer-motion';
import {
  Cpu,
  Thermometer,
  Droplets,
  Wifi,
  Activity,
  TrendingUp,
  Users,
} from 'lucide-react';
import { dashboardApi, devicesApi } from '../services/api';
import { useWebSocket } from '../hooks/useWebSocket';
import type { WebSocketMessage, Device } from '../types';
import DeviceCard from '../components/DeviceCard';

export default function Dashboard() {
  const [realtimeDevices, setRealtimeDevices] = useState<Record<string, boolean>>({});

  const { data: stats } = useQuery({
    queryKey: ['dashboard-stats'],
    queryFn: dashboardApi.getStats,
    refetchInterval: 30000,
  });

  const { data: devices, refetch: refetchDevices } = useQuery({
    queryKey: ['devices'],
    queryFn: devicesApi.getAll,
    refetchInterval: 30000,
  });

  const handleWebSocketMessage = useCallback((message: WebSocketMessage) => {
    if (message.type === 'device_status' && message.device_id) {
      const isOnline = (message.data as { is_online: boolean }).is_online;
      setRealtimeDevices((prev) => ({
        ...prev,
        [message.device_id!]: isOnline,
      }));
    }
    if (message.type === 'metrics') {
      refetchDevices();
    }
  }, [refetchDevices]);

  useWebSocket(handleWebSocketMessage);

  const statCards = [
    {
      label: 'Total Devices',
      value: stats?.total_devices ?? 0,
      icon: Cpu,
      color: 'from-primary-500 to-blue-600',
      bgColor: 'bg-primary-500/10',
    },
    {
      label: 'Online',
      value: stats?.online_devices ?? 0,
      icon: Activity,
      color: 'from-green-400 to-emerald-600',
      bgColor: 'bg-green-500/10',
    },
    {
      label: 'Avg Temperature',
      value: `${(stats?.avg_temperature ?? 0).toFixed(1)}°C`,
      icon: Thermometer,
      color: 'from-orange-400 to-red-500',
      bgColor: 'bg-orange-500/10',
    },
    {
      label: 'Avg Humidity',
      value: `${(stats?.avg_humidity ?? 0).toFixed(1)}%`,
      icon: Droplets,
      color: 'from-cyan-400 to-blue-500',
      bgColor: 'bg-cyan-500/10',
    },
    {
      label: 'Users',
      value: stats?.total_users ?? 0,
      icon: Users,
      color: 'from-purple-400 to-pink-500',
      bgColor: 'bg-purple-500/10',
    },
  ];

  const getDeviceOnlineStatus = (device: Device) => {
    if (device.id in realtimeDevices) {
      return realtimeDevices[device.id];
    }
    return device.is_online;
  };

  return (
    <div className="p-8">
      {/* Header */}
      <motion.div
        initial={{ opacity: 0, y: -20 }}
        animate={{ opacity: 1, y: 0 }}
        className="mb-8"
      >
        <h1 className="text-3xl font-bold mb-2">
          <span className="gradient-text">Dashboard</span>
        </h1>
        <p className="text-dark-400">
          Monitor your IoT mesh network in real-time
        </p>
      </motion.div>

      {/* Stats Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-4 mb-8 stagger-children">
        {statCards.map((stat, index) => (
          <motion.div
            key={stat.label}
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: index * 0.1 }}
            className={`${stat.bgColor} glass rounded-2xl p-5 card-hover`}
          >
            <div className="flex items-start justify-between mb-3">
              <div className={`p-2.5 rounded-xl bg-gradient-to-br ${stat.color}`}>
                <stat.icon className="w-5 h-5 text-white" />
              </div>
              <TrendingUp className="w-4 h-4 text-green-400" />
            </div>
            <p className="text-2xl font-bold text-white mb-1">{stat.value}</p>
            <p className="text-sm text-dark-400">{stat.label}</p>
          </motion.div>
        ))}
      </div>

      {/* Network Status */}
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.5 }}
        className="glass rounded-2xl p-6 mb-8"
      >
        <div className="flex items-center justify-between mb-6">
          <div className="flex items-center gap-3">
            <div className="p-2.5 rounded-xl bg-gradient-to-br from-primary-500 to-accent-400">
              <Wifi className="w-5 h-5 text-white" />
            </div>
            <div>
              <h2 className="text-lg font-semibold">Mesh Network Status</h2>
              <p className="text-sm text-dark-400">Real-time device connections</p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <span className="w-2 h-2 rounded-full bg-green-500 animate-pulse" />
            <span className="text-sm text-dark-300">Live</span>
          </div>
        </div>

        {/* Device Grid */}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {devices?.slice(0, 6).map((device) => (
            <DeviceCard
              key={device.id}
              device={device}
              isOnline={getDeviceOnlineStatus(device)}
            />
          ))}
          {(!devices || devices.length === 0) && (
            <div className="col-span-full text-center py-12">
              <Cpu className="w-12 h-12 text-dark-500 mx-auto mb-4" />
              <p className="text-dark-400">No devices yet</p>
              <p className="text-sm text-dark-500">Add your first ESP device to get started</p>
            </div>
          )}
        </div>
      </motion.div>
    </div>
  );
}


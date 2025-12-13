import { useCallback, useState, useEffect } from 'react';
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
  ChevronRight,
} from 'lucide-react';
import { dashboardApi, devicesApi, metricsApi } from '../services/api';
import { useWebSocket } from '../hooks/useWebSocket';
import { useTranslation } from '../contexts/settingsStore';
import type { WebSocketMessage, Device } from '../types';
import DeviceCard from '../components/DeviceCard';
import MetricChartModal from '../components/MetricChartModal';

type MetricType = 'temperature' | 'humidity' | 'rssi' | 'free_heap';

interface DeviceMetrics {
  temperature?: number;
  humidity?: number;
  rssi?: number;
}

export default function Dashboard() {
  const t = useTranslation();
  const [realtimeDevices, setRealtimeDevices] = useState<Record<string, boolean>>({});
  const [deviceMetrics, setDeviceMetrics] = useState<Record<string, DeviceMetrics>>({});
  const [chartModal, setChartModal] = useState<{ isOpen: boolean; metricType: MetricType }>({
    isOpen: false,
    metricType: 'temperature',
  });

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
    if (message.type === 'metrics' && message.device_id) {
      const data = message.data as { temperature?: number; humidity?: number; rssi?: number };
      setDeviceMetrics((prev) => ({
        ...prev,
        [message.device_id!]: {
          temperature: data.temperature,
          humidity: data.humidity,
          rssi: data.rssi,
        },
      }));
      refetchDevices();
    }
  }, [refetchDevices]);

  useWebSocket(handleWebSocketMessage);

  // Load initial metrics for each device
  useEffect(() => {
    if (devices && devices.length > 0) {
      devices.forEach(async (device) => {
        try {
          const metrics = await metricsApi.getByDeviceId(device.id, 1);
          if (metrics && metrics.length > 0) {
            const latest = metrics[0];
            setDeviceMetrics((prev) => ({
              ...prev,
              [device.id]: {
                temperature: latest.temperature ?? undefined,
                humidity: latest.humidity ?? undefined,
                rssi: latest.rssi ?? undefined,
              },
            }));
          }
        } catch (e) {
          // Ignore errors for individual device metrics
        }
      });
    }
  }, [devices]);

  const openChart = (metricType: MetricType) => {
    setChartModal({ isOpen: true, metricType });
  };

  const statCards = [
    {
      label: t.totalDevices,
      value: stats?.total_devices ?? 0,
      icon: Cpu,
      color: 'from-primary-500 to-blue-600',
      bgColor: 'bg-primary-500/10',
      clickable: false,
    },
    {
      label: t.online,
      value: stats?.online_devices ?? 0,
      icon: Activity,
      color: 'from-green-400 to-emerald-600',
      bgColor: 'bg-green-500/10',
      clickable: false,
    },
    {
      label: t.avgTemperature,
      value: `${Math.round(stats?.avg_temperature ?? 0)}°C`,
      icon: Thermometer,
      color: 'from-orange-400 to-red-500',
      bgColor: 'bg-orange-500/10',
      clickable: true,
      metricType: 'temperature' as MetricType,
    },
    {
      label: t.avgHumidity,
      value: `${(stats?.avg_humidity ?? 0).toFixed(1)}%`,
      icon: Droplets,
      color: 'from-cyan-400 to-blue-500',
      bgColor: 'bg-cyan-500/10',
      clickable: true,
      metricType: 'humidity' as MetricType,
    },
    {
      label: t.users,
      value: stats?.total_users ?? 0,
      icon: Users,
      color: 'from-purple-400 to-pink-500',
      bgColor: 'bg-purple-500/10',
      clickable: false,
    },
  ];

  const getDeviceOnlineStatus = (device: Device) => {
    if (device.id in realtimeDevices) {
      return realtimeDevices[device.id];
    }
    return device.is_online;
  };

  return (
    <div className="p-4 md:p-6 lg:p-8">
      {/* Header */}
      <motion.div
        initial={{ opacity: 0, y: -20 }}
        animate={{ opacity: 1, y: 0 }}
        className="mb-6 md:mb-8"
      >
        <h1 className="text-2xl md:text-3xl font-bold mb-2">
          <span className="gradient-text">{t.dashboard}</span>
        </h1>
        <p className="text-sm md:text-base text-dark-400">
          {t.realtimeConnections}
        </p>
      </motion.div>

      {/* Stats Grid - scrollable on mobile */}
      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-3 lg:grid-cols-5 gap-3 md:gap-4 mb-6 md:mb-8 stagger-children">
        {statCards.map((stat, index) => (
          <motion.div
            key={stat.label}
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: index * 0.1 }}
            onClick={() => stat.clickable && stat.metricType && openChart(stat.metricType)}
            className={`${stat.bgColor} glass rounded-xl md:rounded-2xl p-3 md:p-5 card-hover ${
              stat.clickable 
                ? 'cursor-pointer hover:ring-2 hover:ring-primary-500/50 transition-all group' 
                : ''
            }`}
          >
            <div className="flex items-start justify-between mb-2 md:mb-3">
              <div className={`p-2 md:p-2.5 rounded-lg md:rounded-xl bg-gradient-to-br ${stat.color}`}>
                <stat.icon className="w-4 h-4 md:w-5 md:h-5 text-white" />
              </div>
              {stat.clickable ? (
                <ChevronRight className="w-3 h-3 md:w-4 md:h-4 text-dark-500 group-hover:text-primary-400 transition-colors" />
              ) : (
                <TrendingUp className="w-3 h-3 md:w-4 md:h-4 text-green-400" />
              )}
            </div>
            <p className="text-lg md:text-2xl font-bold text-white mb-0.5 md:mb-1">{stat.value}</p>
            <p className="text-xs md:text-sm text-dark-400 truncate">{stat.label}</p>
            {stat.clickable && (
              <p className="text-xs text-dark-500 mt-1 md:mt-2 group-hover:text-primary-400 transition-colors hidden md:block">
                →
              </p>
            )}
          </motion.div>
        ))}
      </div>

      {/* Network Status */}
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.5 }}
        className="glass rounded-xl md:rounded-2xl p-4 md:p-6 mb-6 md:mb-8"
      >
        <div className="flex items-center justify-between mb-4 md:mb-6">
          <div className="flex items-center gap-2 md:gap-3">
            <div className="p-2 md:p-2.5 rounded-lg md:rounded-xl bg-gradient-to-br from-primary-500 to-accent-400">
              <Wifi className="w-4 h-4 md:w-5 md:h-5 text-white" />
            </div>
            <div>
              <h2 className="text-base md:text-lg font-semibold">{t.meshNetworkStatus}</h2>
              <p className="text-xs md:text-sm text-dark-400 hidden sm:block">{t.realtimeConnections}</p>
            </div>
          </div>
          <div className="flex items-center gap-1.5 md:gap-2">
            <span className="w-2 h-2 rounded-full bg-green-500 animate-pulse" />
            <span className="text-xs md:text-sm text-dark-300">{t.live}</span>
          </div>
        </div>

        {/* Device Grid */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 md:gap-4">
          {devices?.slice(0, 6).map((device) => (
            <DeviceCard
              key={device.id}
              device={device}
              isOnline={getDeviceOnlineStatus(device)}
              temperature={deviceMetrics[device.id]?.temperature}
              humidity={deviceMetrics[device.id]?.humidity}
              rssi={deviceMetrics[device.id]?.rssi}
            />
          ))}
          {(!devices || devices.length === 0) && (
            <div className="col-span-full text-center py-12">
              <Cpu className="w-12 h-12 text-dark-500 mx-auto mb-4" />
              <p className="text-dark-400">{t.noDevicesYet}</p>
              <p className="text-sm text-dark-500">{t.addFirstDevice}</p>
            </div>
          )}
        </div>
      </motion.div>

      {/* Metric Chart Modal */}
      <MetricChartModal
        isOpen={chartModal.isOpen}
        onClose={() => setChartModal({ ...chartModal, isOpen: false })}
        metricType={chartModal.metricType}
      />
    </div>
  );
}

import { useQuery } from '@tanstack/react-query';
import { motion, AnimatePresence } from 'framer-motion';
import { X, Thermometer, Droplets, Wifi, Cpu } from 'lucide-react';
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler,
  TooltipItem,
} from 'chart.js';
import { Line } from 'react-chartjs-2';
import { format } from 'date-fns';
import { devicesApi, metricsApi } from '../services/api';
import { useSettingsStore } from '../contexts/settingsStore';
import type { Metric } from '../types';
import { useState } from 'react';

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler
);

type MetricType = 'temperature' | 'humidity' | 'rssi' | 'free_heap';

interface MetricChartModalProps {
  isOpen: boolean;
  onClose: () => void;
  metricType: MetricType;
}

const metricStyles = {
  temperature: {
    unit: '°C',
    icon: Thermometer,
    color: '#f97316',
    bgColor: 'rgba(249, 115, 22, 0.1)',
    gradient: 'from-orange-500 to-red-500',
  },
  humidity: {
    unit: '%',
    icon: Droplets,
    color: '#06b6d4',
    bgColor: 'rgba(6, 182, 212, 0.1)',
    gradient: 'from-cyan-500 to-blue-500',
  },
  rssi: {
    unit: 'dBm',
    icon: Wifi,
    color: '#a855f7',
    bgColor: 'rgba(168, 85, 247, 0.1)',
    gradient: 'from-purple-500 to-pink-500',
  },
  free_heap: {
    unit: 'KB',
    icon: Cpu,
    color: '#22c55e',
    bgColor: 'rgba(34, 197, 94, 0.1)',
    gradient: 'from-green-500 to-emerald-500',
  },
};

export default function MetricChartModal({ isOpen, onClose, metricType }: MetricChartModalProps) {
  const [selectedPeriod, setSelectedPeriod] = useState('1h');
  const [selectedDeviceId, setSelectedDeviceId] = useState<string | 'all'>('all');
  const { language } = useSettingsStore();
  
  const labels = {
    uk: {
      temperature: 'Температура',
      humidity: 'Вологість',
      rssi: 'WiFi Сигнал',
      free_heap: "Вільна пам'ять",
      history: 'Історія показників',
      period: 'Період',
      device: 'Пристрій',
      allDevices: 'Всі пристрої',
      current: 'Поточне',
      min: 'Мінімум',
      max: 'Максимум',
      avg: 'Середнє',
      noData: 'Немає даних за обраний період',
      connectDevice: 'Підключіть ESP пристрій та почніть збирати метрики',
      hour: 'год',
      days: 'днів',
    },
    en: {
      temperature: 'Temperature',
      humidity: 'Humidity',
      rssi: 'WiFi Signal',
      free_heap: 'Free Memory',
      history: 'History',
      period: 'Period',
      device: 'Device',
      allDevices: 'All Devices',
      current: 'Current',
      min: 'Min',
      max: 'Max',
      avg: 'Average',
      noData: 'No data for selected period',
      connectDevice: 'Connect ESP device and start collecting metrics',
      hour: 'h',
      days: 'days',
    },
  };
  
  const t = labels[language];
  const style = metricStyles[metricType];
  const config = {
    ...style,
    label: t[metricType],
  };

  const { data: devices } = useQuery({
    queryKey: ['devices'],
    queryFn: devicesApi.getAll,
    enabled: isOpen,
  });

  const { data: allMetrics, isLoading } = useQuery({
    queryKey: ['all-metrics', selectedPeriod, selectedDeviceId],
    queryFn: async () => {
      if (!devices || devices.length === 0) return [];
      
      const devicesToFetch = selectedDeviceId === 'all' 
        ? devices 
        : devices.filter(d => d.id === selectedDeviceId);
      
      const metricsPromises = devicesToFetch.map(device =>
        metricsApi.getByPeriod(device.id, selectedPeriod)
          .then(metrics => metrics.map(m => ({ ...m, deviceName: device.name })))
          .catch(() => [])
      );
      
      const results = await Promise.all(metricsPromises);
      return results.flat().sort((a, b) => 
        new Date(a.created_at).getTime() - new Date(b.created_at).getTime()
      );
    },
    enabled: isOpen && !!devices && devices.length > 0,
    refetchInterval: 30000,
  });

  const periods = [
    { value: '1h', label: `1 ${t.hour}` },
    { value: '6h', label: `6 ${t.hour}` },
    { value: '24h', label: `24 ${t.hour}` },
    { value: '168h', label: `7 ${t.days}` },
  ];

  const getMetricValue = (metric: Metric & { deviceName?: string }) => {
    switch (metricType) {
      case 'temperature':
        return metric.temperature;
      case 'humidity':
        return metric.humidity;
      case 'rssi':
        return metric.rssi;
      case 'free_heap':
        return metric.free_heap ? metric.free_heap / 1024 : null;
      default:
        return null;
    }
  };

  // Group metrics by device for multi-line chart
  const groupedMetrics = allMetrics?.reduce((acc, metric) => {
    const deviceName = (metric as Metric & { deviceName: string }).deviceName || 'Unknown';
    if (!acc[deviceName]) {
      acc[deviceName] = [];
    }
    acc[deviceName].push(metric);
    return acc;
  }, {} as Record<string, Metric[]>) || {};

  const deviceColors = [
    { border: '#f97316', bg: 'rgba(249, 115, 22, 0.1)' },
    { border: '#06b6d4', bg: 'rgba(6, 182, 212, 0.1)' },
    { border: '#a855f7', bg: 'rgba(168, 85, 247, 0.1)' },
    { border: '#22c55e', bg: 'rgba(34, 197, 94, 0.1)' },
    { border: '#ec4899', bg: 'rgba(236, 72, 153, 0.1)' },
    { border: '#eab308', bg: 'rgba(234, 179, 8, 0.1)' },
  ];

  const chartData = {
    labels: allMetrics?.map((m) => format(new Date(m.created_at), 'HH:mm')) || [],
    datasets: selectedDeviceId === 'all' && Object.keys(groupedMetrics).length > 1
      ? Object.entries(groupedMetrics).map(([deviceName, metrics], index) => ({
          label: deviceName,
          data: metrics.map(m => getMetricValue(m as Metric & { deviceName: string }) ?? null),
          borderColor: deviceColors[index % deviceColors.length].border,
          backgroundColor: deviceColors[index % deviceColors.length].bg,
          fill: false,
          tension: 0.4,
          pointRadius: 2,
          spanGaps: false, // Don't connect line over null/missing values
        }))
      : [{
          label: config.label,
          data: allMetrics?.map(m => getMetricValue(m as Metric & { deviceName: string }) ?? null) || [],
          borderColor: config.color,
          backgroundColor: config.bgColor,
          fill: true,
          tension: 0.4,
          pointRadius: 3,
          pointHoverRadius: 6,
          spanGaps: false, // Don't connect line over null/missing values
        }],
  };

  const chartOptions = {
    responsive: true,
    maintainAspectRatio: false,
    interaction: {
      mode: 'index' as const,
      intersect: false,
    },
    plugins: {
      legend: {
        display: selectedDeviceId === 'all' && Object.keys(groupedMetrics).length > 1,
        position: 'top' as const,
        labels: { color: '#9ca3af' },
      },
      tooltip: {
        backgroundColor: 'rgba(17, 24, 39, 0.9)',
        titleColor: '#fff',
        bodyColor: '#9ca3af',
        borderColor: 'rgba(255, 255, 255, 0.1)',
        borderWidth: 1,
        padding: 12,
        displayColors: true,
        callbacks: {
          label: (context: TooltipItem<'line'>) => {
            const value = context.parsed.y;
            const label = context.dataset.label || config.label;
            return `${label}: ${value?.toFixed(1) ?? '--'} ${config.unit}`;
          },
        },
      },
    },
    scales: {
      x: {
        grid: { color: 'rgba(255,255,255,0.05)' },
        ticks: { color: '#6b7280', maxRotation: 45 },
      },
      y: {
        grid: { color: 'rgba(255,255,255,0.05)' },
        ticks: { 
          color: '#6b7280',
          callback: (value: string | number) => `${value}${config.unit}`,
        },
      },
    },
  };

  // Calculate stats
  const values = allMetrics?.map(m => getMetricValue(m as Metric & { deviceName: string })).filter((v): v is number => v !== null) || [];
  const stats = {
    current: values[values.length - 1] ?? null,
    min: values.length > 0 ? Math.min(...values) : null,
    max: values.length > 0 ? Math.max(...values) : null,
    avg: values.length > 0 ? values.reduce((a, b) => a + b, 0) / values.length : null,
  };

  return (
    <AnimatePresence>
      {isOpen && (
        <>
          {/* Backdrop */}
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            onClick={onClose}
            className="fixed inset-0 bg-black/60 backdrop-blur-sm z-50"
          />
          
          {/* Modal */}
          <motion.div
            initial={{ opacity: 0, scale: 0.95, y: 20 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.95, y: 20 }}
            className="fixed inset-4 md:inset-10 lg:inset-20 bg-dark-800 rounded-2xl shadow-2xl z-50 overflow-hidden flex flex-col"
          >
            {/* Header */}
            <div className="flex items-center justify-between p-6 border-b border-dark-700">
              <div className="flex items-center gap-4">
                <div className={`p-3 rounded-xl bg-gradient-to-br ${config.gradient}`}>
                  <config.icon className="w-6 h-6 text-white" />
                </div>
                <div>
                  <h2 className="text-xl font-bold">{config.label}</h2>
                  <p className="text-sm text-dark-400">{t.history}</p>
                </div>
              </div>
              <button
                onClick={onClose}
                className="p-2 rounded-lg hover:bg-dark-700 transition-colors"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            {/* Controls */}
            <div className="flex flex-wrap items-center gap-4 p-4 border-b border-dark-700/50">
              {/* Period selector */}
              <div className="flex items-center gap-2">
                <span className="text-sm text-dark-400">{t.period}:</span>
                <div className="flex bg-dark-700/50 rounded-lg p-1">
                  {periods.map((p) => (
                    <button
                      key={p.value}
                      onClick={() => setSelectedPeriod(p.value)}
                      className={`px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
                        selectedPeriod === p.value
                          ? 'bg-primary-500 text-white'
                          : 'text-dark-400 hover:text-white'
                      }`}
                    >
                      {p.label}
                    </button>
                  ))}
                </div>
              </div>

              {/* Device selector */}
              {devices && devices.length > 1 && (
                <div className="flex items-center gap-2">
                  <span className="text-sm text-dark-400">{t.device}:</span>
                  <select
                    value={selectedDeviceId}
                    onChange={(e) => setSelectedDeviceId(e.target.value)}
                    className="bg-dark-700/50 border border-dark-600 rounded-lg px-3 py-1.5 text-sm"
                  >
                    <option value="all">{t.allDevices}</option>
                    {devices.map((device) => (
                      <option key={device.id} value={device.id}>
                        {device.name}
                      </option>
                    ))}
                  </select>
                </div>
              )}
            </div>

            {/* Stats */}
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4 p-4">
              {[
                { label: t.current, value: stats.current },
                { label: t.min, value: stats.min },
                { label: t.max, value: stats.max },
                { label: t.avg, value: stats.avg },
              ].map((stat) => (
                <div key={stat.label} className="bg-dark-700/30 rounded-xl p-4">
                  <p className="text-sm text-dark-400 mb-1">{stat.label}</p>
                  <p className="text-2xl font-bold">
                    {stat.value !== null ? stat.value.toFixed(1) : '--'}
                    <span className="text-sm text-dark-500 ml-1">{config.unit}</span>
                  </p>
                </div>
              ))}
            </div>

            {/* Chart */}
            <div className="flex-1 p-4 min-h-0">
              {isLoading ? (
                <div className="flex items-center justify-center h-full">
                  <div className="w-12 h-12 spinner" />
                </div>
              ) : !allMetrics || allMetrics.length === 0 ? (
                <div className="flex flex-col items-center justify-center h-full text-dark-400">
                  <config.icon className="w-16 h-16 mb-4 opacity-20" />
                  <p>{t.noData}</p>
                  <p className="text-sm text-dark-500 mt-1">
                    {t.connectDevice}
                  </p>
                </div>
              ) : (
                <div className="h-full">
                  <Line data={chartData} options={chartOptions} />
                </div>
              )}
            </div>
          </motion.div>
        </>
      )}
    </AnimatePresence>
  );
}


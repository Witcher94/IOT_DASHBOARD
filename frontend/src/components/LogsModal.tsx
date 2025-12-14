import { useState } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { X, Terminal, Server, RefreshCw, Wifi } from 'lucide-react';
import { format } from 'date-fns';

interface LogEntry {
  timestamp: string;
  level: string;
  message: string;
}

interface LogsModalProps {
  isOpen: boolean;
  onClose: () => void;
  gatewayName: string;
  gatewayIp?: string;
}

export default function LogsModal({ isOpen, onClose, gatewayName, gatewayIp }: LogsModalProps) {
  const [activeTab, setActiveTab] = useState<'serial' | 'gateway'>('serial');
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(false);

  const gatewayUrl = gatewayIp ? `http://${gatewayIp}:8080` : 'http://192.168.0.92:8080';

  const fetchLogs = async (type: 'serial' | 'gateway') => {
    setLoading(true);
    try {
      const response = await fetch(`${gatewayUrl}/api/logs/${type}`);
      if (response.ok) {
        const data = await response.json();
        setLogs(data || []);
      }
    } catch (error) {
      console.error('Failed to fetch logs:', error);
      setLogs([]);
    }
    setLoading(false);
  };

  const handleTabChange = (tab: 'serial' | 'gateway') => {
    setActiveTab(tab);
    fetchLogs(tab);
  };

  const getLevelColor = (level: string) => {
    switch (level.toLowerCase()) {
      case 'error':
        return 'text-red-400';
      case 'warn':
      case 'warning':
        return 'text-yellow-400';
      case 'info':
        return 'text-blue-400';
      case 'data':
        return 'text-green-400';
      default:
        return 'text-dark-300';
    }
  };

  if (!isOpen) return null;

  return (
    <AnimatePresence>
      <motion.div
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        exit={{ opacity: 0 }}
        className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4"
        onClick={onClose}
      >
        <motion.div
          initial={{ scale: 0.9, opacity: 0 }}
          animate={{ scale: 1, opacity: 1 }}
          exit={{ scale: 0.9, opacity: 0 }}
          onClick={(e) => e.stopPropagation()}
          className="w-full max-w-4xl h-[80vh] bg-dark-800 rounded-2xl border border-dark-700 flex flex-col overflow-hidden"
        >
          {/* Header */}
          <div className="flex items-center justify-between p-4 border-b border-dark-700">
            <div className="flex items-center gap-3">
              <Terminal className="w-5 h-5 text-primary" />
              <h2 className="text-lg font-semibold">{gatewayName} Logs</h2>
            </div>
            <div className="flex items-center gap-2">
              <button
                onClick={() => fetchLogs(activeTab)}
                className="p-2 hover:bg-dark-700 rounded-lg transition-colors"
                title="Refresh"
              >
                <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
              </button>
              <button
                onClick={onClose}
                className="p-2 hover:bg-dark-700 rounded-lg transition-colors"
              >
                <X className="w-5 h-5" />
              </button>
            </div>
          </div>

          {/* Tabs */}
          <div className="flex border-b border-dark-700">
            <button
              onClick={() => handleTabChange('serial')}
              className={`flex-1 px-4 py-3 flex items-center justify-center gap-2 transition-colors ${
                activeTab === 'serial'
                  ? 'bg-dark-700 text-primary border-b-2 border-primary'
                  : 'text-dark-400 hover:text-white'
              }`}
            >
              <Wifi className="w-4 h-4" />
              Serial (Mesh Data)
            </button>
            <button
              onClick={() => handleTabChange('gateway')}
              className={`flex-1 px-4 py-3 flex items-center justify-center gap-2 transition-colors ${
                activeTab === 'gateway'
                  ? 'bg-dark-700 text-primary border-b-2 border-primary'
                  : 'text-dark-400 hover:text-white'
              }`}
            >
              <Server className="w-4 h-4" />
              Gateway Logs
            </button>
          </div>

          {/* Logs Content */}
          <div className="flex-1 overflow-auto p-4 font-mono text-sm bg-dark-900">
            {loading ? (
              <div className="flex items-center justify-center h-full">
                <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
              </div>
            ) : logs.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-full text-dark-400">
                <Terminal className="w-12 h-12 mb-4 opacity-50" />
                <p>No logs available</p>
                <p className="text-xs mt-2">
                  Make sure gateway is running at{' '}
                  <a
                    href={gatewayUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-primary hover:underline"
                  >
                    {gatewayUrl}
                  </a>
                </p>
              </div>
            ) : (
              <div className="space-y-1">
                {logs.slice().reverse().map((log, index) => (
                  <div key={index} className="flex gap-3 hover:bg-dark-800/50 px-2 py-1 rounded">
                    <span className="text-dark-500 whitespace-nowrap">
                      {format(new Date(log.timestamp), 'HH:mm:ss')}
                    </span>
                    <span className={`w-12 ${getLevelColor(log.level)}`}>
                      [{log.level.toUpperCase()}]
                    </span>
                    <span className="text-dark-200 break-all">{log.message}</span>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Footer */}
          <div className="p-3 border-t border-dark-700 text-xs text-dark-400 flex items-center justify-between">
            <span>
              Gateway: <span className="text-dark-300">{gatewayUrl}</span>
            </span>
            <span>{logs.length} entries</span>
          </div>
        </motion.div>
      </motion.div>
    </AnimatePresence>
  );
}


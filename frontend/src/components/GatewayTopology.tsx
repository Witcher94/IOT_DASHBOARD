import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { gatewayApi, metricsApi } from '../services/api';
import type { GatewayTopology, Device } from '../types';
import { Cpu, Wifi, Activity, RefreshCw, Power, Thermometer, HardDrive, Server, Radio, Terminal } from 'lucide-react';
import { motion } from 'framer-motion';
import { formatDistanceToNow } from 'date-fns';
import { toast } from 'react-hot-toast';
import LogsModal from './LogsModal';

interface GatewayTopologyProps {
  gatewayId: string;
}

export default function GatewayTopology({ gatewayId }: GatewayTopologyProps) {
  const [showLogs, setShowLogs] = useState(false);
  
  const { data: topology, isLoading, refetch } = useQuery({
    queryKey: ['gateway-topology', gatewayId],
    queryFn: () => gatewayApi.getTopology(gatewayId),
    refetchInterval: 10000,
  });

  const { data: gatewayMetrics } = useQuery({
    queryKey: ['gateway-metrics', gatewayId],
    queryFn: () => metricsApi.getByDeviceId(gatewayId, 1),
    refetchInterval: 10000,
    enabled: !!gatewayId,
  });

  const latestMetric = gatewayMetrics?.[0];

  const handleCommand = async (nodeId: string, command: string) => {
    try {
      await gatewayApi.sendCommandToNode(gatewayId, nodeId, { command });
      toast.success(`Command "${command}" sent`);
      setTimeout(() => refetch(), 1000);
    } catch (error: any) {
      toast.error(error.response?.data?.error || 'Failed to send command');
    }
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="w-8 h-8 spinner" />
      </div>
    );
  }

  if (!topology) {
    return <div className="text-dark-400">Failed to load topology</div>;
  }

  // Find bridge node (first node, usually ESP32 connected via serial)
  const bridgeNode = topology.mesh_nodes.find(n => 
    n.name?.toLowerCase().includes('bridge') || 
    n.mesh_node_id === topology.mesh_nodes[0]?.mesh_node_id
  ) || topology.mesh_nodes[0];
  
  // Other mesh nodes (excluding bridge)
  const meshNodes = topology.mesh_nodes.filter(n => n.id !== bridgeNode?.id);

  return (
    <div className="space-y-6">
      {/* Gateway Health Stats */}
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        className="bg-dark-800/50 backdrop-blur rounded-2xl p-6 border border-dark-700/50"
      >
        <div className="flex items-center justify-between mb-5">
          <h3 className="text-lg font-semibold text-white flex items-center gap-2">
            <Server className="w-5 h-5 text-primary" />
            Gateway Health
          </h3>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setShowLogs(true)}
              className="flex items-center gap-1.5 px-3 py-1.5 bg-dark-700 hover:bg-dark-600 rounded-lg transition-colors text-dark-300 hover:text-white text-sm"
            >
              <Terminal className="w-4 h-4" />
              Logs
            </button>
            <button
              onClick={() => refetch()}
              className="p-2 hover:bg-dark-700 rounded-lg transition-colors text-dark-400 hover:text-white"
            >
              <RefreshCw className="w-4 h-4" />
            </button>
          </div>
        </div>
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-4">
          <div className="bg-dark-900/50 rounded-xl p-3">
            <div className="text-xs text-dark-400 mb-1">Status</div>
            <div className={`text-base font-semibold ${topology.gateway.is_online ? 'text-green-400' : 'text-red-400'}`}>
              {topology.gateway.is_online ? '● Online' : '○ Offline'}
            </div>
          </div>
          <div className="bg-dark-900/50 rounded-xl p-3">
            <div className="text-xs text-dark-400 mb-1 flex items-center gap-1">
              <Cpu className="w-3 h-3" /> CPU
            </div>
            <div className="text-base font-semibold text-cyan-400">
              {latestMetric?.free_heap != null ? `${(latestMetric.free_heap / 10).toFixed(1)}%` : '--'}
            </div>
          </div>
          <div className="bg-dark-900/50 rounded-xl p-3">
            <div className="text-xs text-dark-400 mb-1 flex items-center gap-1">
              <HardDrive className="w-3 h-3" /> RAM
            </div>
            <div className="text-base font-semibold text-blue-400">
              {latestMetric?.humidity ? `${latestMetric.humidity.toFixed(0)}%` : '--'}
            </div>
          </div>
          <div className="bg-dark-900/50 rounded-xl p-3">
            <div className="text-xs text-dark-400 mb-1 flex items-center gap-1">
              <Thermometer className="w-3 h-3" /> Temp
            </div>
            <div className="text-base font-semibold text-orange-400">
              {latestMetric?.temperature ? `${Math.round(latestMetric.temperature)}°C` : '--'}
            </div>
          </div>
          <div className="bg-dark-900/50 rounded-xl p-3">
            <div className="text-xs text-dark-400 mb-1">Nodes</div>
            <div className="text-base font-semibold text-purple-400">
              {topology.online_nodes}/{topology.total_nodes}
            </div>
          </div>
          <div className="bg-dark-900/50 rounded-xl p-3">
            <div className="text-xs text-dark-400 mb-1">Last Seen</div>
            <div className="text-sm text-dark-300">
              {topology.gateway.last_seen
                ? formatDistanceToNow(new Date(topology.gateway.last_seen), { addSuffix: true })
                : 'Never'}
            </div>
          </div>
        </div>
      </motion.div>

      {/* Mesh Topology Visualization */}
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.1 }}
        className="bg-dark-800/50 backdrop-blur rounded-2xl p-6 border border-dark-700/50 overflow-hidden"
      >
        <h3 className="text-lg font-semibold text-white mb-8 flex items-center gap-2">
          <Radio className="w-5 h-5 text-primary" />
          Mesh Network Topology
        </h3>

        {/* Topology Tree */}
        <div className="relative min-h-[400px]">
          {/* SVG for connection lines */}
          <svg className="absolute inset-0 w-full h-full pointer-events-none" style={{ zIndex: 0 }}>
            <defs>
              <marker id="arrowhead" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
                <polygon points="0 0, 10 3.5, 0 7" fill="#4B5563" />
              </marker>
            </defs>
            {/* Gateway to Bridge line */}
            {bridgeNode && (
              <line 
                x1="50%" y1="80" x2="50%" y2="160" 
                stroke="#4B5563" strokeWidth="2" 
                markerEnd="url(#arrowhead)"
              />
            )}
            {/* Bridge to Mesh nodes lines */}
            {meshNodes.length > 0 && (
              <line 
                x1="50%" y1="240" x2="50%" y2="300" 
                stroke="#4B5563" strokeWidth="2" 
                markerEnd="url(#arrowhead)"
              />
            )}
          </svg>

          {/* Gateway Node (RPi) */}
          <motion.div
            initial={{ opacity: 0, scale: 0.9 }}
            animate={{ opacity: 1, scale: 1 }}
            className="relative flex justify-center mb-8"
            style={{ zIndex: 1 }}
          >
            <div className={`
              bg-gradient-to-br from-slate-100 to-slate-200 
              border-2 ${topology.gateway.is_online ? 'border-green-500' : 'border-gray-400'}
              rounded-xl px-8 py-4 min-w-[180px] text-center shadow-xl
            `}>
              <Server className="w-8 h-8 mx-auto mb-2 text-slate-700" />
              <div className="font-bold text-slate-800 text-lg">{topology.gateway.name}</div>
              <div className="text-xs text-slate-500 mt-1">Raspberry Pi Gateway</div>
              <div className={`text-xs mt-2 font-medium ${topology.gateway.is_online ? 'text-green-600' : 'text-red-500'}`}>
                {topology.gateway.is_online ? '● Online' : '○ Offline'}
              </div>
            </div>
          </motion.div>

          {/* Bridge Node (ESP32 connected via USB) */}
          {bridgeNode && (
            <motion.div
              initial={{ opacity: 0, scale: 0.9 }}
              animate={{ opacity: 1, scale: 1 }}
              transition={{ delay: 0.15 }}
              className="relative flex justify-center mb-8"
              style={{ zIndex: 1 }}
            >
              <Link to={`/devices/${bridgeNode.id}`}>
                <motion.div
                  whileHover={{ scale: 1.03 }}
                  className={`
                    bg-gradient-to-br from-dark-700 to-dark-800
                    border-2 ${bridgeNode.is_online ? 'border-cyan-500' : 'border-dark-600'}
                    rounded-xl px-8 py-4 min-w-[160px] text-center shadow-xl cursor-pointer
                    hover:shadow-cyan-500/20 transition-shadow
                  `}
                >
                  <Wifi className="w-7 h-7 mx-auto mb-2 text-cyan-400" />
                  <div className="font-semibold text-white">{bridgeNode.name || 'Bridge'}</div>
                  <div className="text-xs text-dark-400 mt-1">ESP32 Bridge</div>
                  <div className={`text-xs mt-2 ${bridgeNode.is_online ? 'text-cyan-400' : 'text-dark-500'}`}>
                    {bridgeNode.is_online ? '● Online' : '○ Offline'}
                  </div>
                </motion.div>
              </Link>
            </motion.div>
          )}

          {/* Mesh Nodes Row */}
          {meshNodes.length > 0 ? (
            <motion.div
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: 0.25 }}
              className="relative flex justify-center gap-4 flex-wrap"
              style={{ zIndex: 1 }}
            >
              {/* Horizontal connection line between mesh nodes */}
              {meshNodes.length > 1 && (
                <div className="absolute top-1/2 left-1/4 right-1/4 h-0.5 bg-dark-600 -translate-y-1/2" style={{ zIndex: 0 }} />
              )}
              
              {meshNodes.map((node: Device, index: number) => (
                <motion.div
                  key={node.id}
                  initial={{ opacity: 0, y: 20 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ delay: 0.3 + index * 0.1 }}
                  className="relative"
                >
                  <Link to={`/devices/${node.id}`}>
                    <motion.div
                      whileHover={{ scale: 1.05 }}
                      className={`
                        bg-gradient-to-br from-dark-700 to-dark-800
                        border-2 ${node.is_online ? 'border-green-500' : 'border-dark-600'}
                        rounded-xl px-6 py-4 min-w-[140px] text-center shadow-lg cursor-pointer
                        hover:shadow-green-500/20 transition-shadow
                      `}
                    >
                      <Activity className={`w-6 h-6 mx-auto mb-2 ${node.is_online ? 'text-green-400' : 'text-dark-500'}`} />
                      <div className="font-semibold text-white text-sm">{node.name}</div>
                      <div className="text-xs text-dark-400 mt-1">
                        {node.platform || 'ESP'} Node
                      </div>
                      <div className={`text-xs mt-2 ${node.is_online ? 'text-green-400' : 'text-dark-500'}`}>
                        {node.is_online ? '● Online' : '○ Offline'}
                      </div>
                    </motion.div>
                  </Link>
                  
                  {/* Quick actions */}
                  <div className="flex gap-1 mt-2 justify-center">
                    <button
                      onClick={() => handleCommand(node.id, 'status')}
                      className="p-1.5 bg-dark-700 hover:bg-dark-600 rounded-lg transition-colors"
                      title="Refresh"
                    >
                      <RefreshCw className="w-3 h-3 text-dark-400" />
                    </button>
                    <button
                      onClick={() => {
                        if (confirm(`Reboot "${node.name}"?`)) {
                          handleCommand(node.id, 'reboot');
                        }
                      }}
                      className="p-1.5 bg-dark-700 hover:bg-red-500/20 rounded-lg transition-colors"
                      title="Reboot"
                    >
                      <Power className="w-3 h-3 text-dark-400 hover:text-red-400" />
                    </button>
                  </div>
                </motion.div>
              ))}
            </motion.div>
          ) : !bridgeNode ? (
            <div className="text-center py-12 text-dark-400">
              <Wifi className="w-16 h-16 mx-auto mb-4 opacity-30" />
              <p className="text-lg">No mesh nodes connected</p>
              <p className="text-sm text-dark-500 mt-2">
                Connect ESP32/ESP8266 nodes to see them here
              </p>
            </div>
          ) : null}
        </div>

        {/* Legend */}
        <div className="mt-8 pt-4 border-t border-dark-700/50">
          <div className="flex flex-wrap gap-6 justify-center text-xs text-dark-400">
            <div className="flex items-center gap-2">
              <div className="w-3 h-3 rounded-full bg-green-500" />
              <span>Online</span>
            </div>
            <div className="flex items-center gap-2">
              <div className="w-3 h-3 rounded-full bg-dark-600" />
              <span>Offline</span>
            </div>
            <div className="flex items-center gap-2">
              <Server className="w-4 h-4 text-slate-400" />
              <span>Gateway (RPi)</span>
            </div>
            <div className="flex items-center gap-2">
              <Wifi className="w-4 h-4 text-cyan-400" />
              <span>Bridge (ESP32)</span>
            </div>
            <div className="flex items-center gap-2">
              <Activity className="w-4 h-4 text-green-400" />
              <span>Mesh Node</span>
            </div>
          </div>
        </div>
      </motion.div>

      {/* Logs Modal */}
      <LogsModal
        isOpen={showLogs}
        onClose={() => setShowLogs(false)}
        gatewayName={topology?.gateway?.name || 'Gateway'}
        gatewayIp="192.168.0.92"
      />
    </div>
  );
}

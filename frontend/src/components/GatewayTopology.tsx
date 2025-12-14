import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { gatewayApi, metricsApi } from '../services/api';
import type { GatewayTopology, Device } from '../types';
import { Cpu, Wifi, Activity, RefreshCw, Power, Thermometer, HardDrive, ExternalLink } from 'lucide-react';
import { motion } from 'framer-motion';
import { formatDistanceToNow } from 'date-fns';
import { toast } from 'react-hot-toast';

interface GatewayTopologyProps {
  gatewayId: string;
}

export default function GatewayTopology({ gatewayId }: GatewayTopologyProps) {
  const { data: topology, isLoading, refetch } = useQuery({
    queryKey: ['gateway-topology', gatewayId],
    queryFn: () => gatewayApi.getTopology(gatewayId),
    refetchInterval: 10000, // Refresh every 10s
  });

  // Get latest gateway metrics for CPU, Memory, Temp
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
      toast.success(`Command "${command}" sent to node`);
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

  return (
    <div className="space-y-6">
      {/* Gateway Health */}
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        className="bg-dark-800 rounded-xl p-6 border border-dark-700"
      >
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-lg font-semibold text-primary flex items-center gap-2">
            <Cpu className="w-5 h-5" />
            Gateway Health
          </h3>
          <button
            onClick={() => refetch()}
            className="p-2 hover:bg-dark-700 rounded-lg transition-colors"
          >
            <RefreshCw className="w-4 h-4" />
          </button>
        </div>
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
          <div>
            <div className="text-sm text-dark-400 mb-1">Status</div>
            <div className={`text-lg font-semibold ${topology.gateway.is_online ? 'text-success' : 'text-danger'}`}>
              {topology.gateway.is_online ? '🟢 Online' : '🔴 Offline'}
            </div>
          </div>
          <div>
            <div className="text-sm text-dark-400 mb-1 flex items-center gap-1">
              <Cpu className="w-3 h-3" /> CPU
            </div>
            <div className="text-lg font-semibold text-primary">
              {latestMetric?.free_heap ? `${(latestMetric.free_heap / 10).toFixed(1)}%` : '--'}
            </div>
          </div>
          <div>
            <div className="text-sm text-dark-400 mb-1 flex items-center gap-1">
              <HardDrive className="w-3 h-3" /> Memory
            </div>
            <div className="text-lg font-semibold text-primary">
              {latestMetric?.humidity ? `${latestMetric.humidity.toFixed(1)}%` : '--'}
            </div>
          </div>
          <div>
            <div className="text-sm text-dark-400 mb-1 flex items-center gap-1">
              <Thermometer className="w-3 h-3" /> CPU Temp
            </div>
            <div className="text-lg font-semibold text-orange-400">
              {latestMetric?.temperature ? `${Math.round(latestMetric.temperature)}°C` : '--'}
            </div>
          </div>
          <div>
            <div className="text-sm text-dark-400 mb-1">Mesh Nodes</div>
            <div className="text-lg font-semibold text-primary">
              {topology.online_nodes}/{topology.total_nodes}
            </div>
          </div>
          <div>
            <div className="text-sm text-dark-400 mb-1">Last Seen</div>
            <div className="text-sm text-dark-300">
              {topology.gateway.last_seen
                ? formatDistanceToNow(new Date(topology.gateway.last_seen), { addSuffix: true })
                : 'Never'}
            </div>
          </div>
        </div>
      </motion.div>

      {/* Tree Topology */}
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.1 }}
        className="bg-dark-800 rounded-xl p-6 border border-dark-700"
      >
        <h3 className="text-lg font-semibold text-primary mb-6 flex items-center gap-2">
          <Wifi className="w-5 h-5" />
          Mesh Network Topology
        </h3>

        {/* Gateway (Root) */}
        <div className="flex flex-col items-center mb-8">
          <div className="relative">
            <motion.div
              whileHover={{ scale: 1.05 }}
              className={`bg-gradient-to-br ${
                topology.gateway.is_online
                  ? 'from-primary/20 to-primary/10 border-primary'
                  : 'from-dark-700 to-dark-800 border-dark-600'
              } border-2 rounded-xl p-4 min-w-[200px] text-center`}
            >
              <Cpu className="w-8 h-8 mx-auto mb-2 text-primary" />
              <div className="font-semibold text-dark-100">{topology.gateway.name}</div>
              <div className="text-xs text-dark-400 mt-1">Gateway</div>
              <div className={`text-xs mt-2 ${topology.gateway.is_online ? 'text-success' : 'text-danger'}`}>
                {topology.gateway.is_online ? '🟢 Online' : '🔴 Offline'}
              </div>
            </motion.div>
          </div>

          {/* Connection Line */}
          {topology.mesh_nodes.length > 0 && (
            <div className="w-0.5 h-8 bg-dark-600 my-2" />
          )}
        </div>

        {/* Mesh Nodes */}
        {topology.mesh_nodes.length > 0 ? (
          <div className="flex justify-center">
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
              {topology.mesh_nodes.map((node: Device, index: number) => (
                <motion.div
                  key={node.id}
                  initial={{ opacity: 0, y: 20 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ delay: 0.2 + index * 0.05 }}
                  className="relative w-[220px]"
                >
                  {/* Connection Line */}
                  <div className="absolute -top-4 left-1/2 -translate-x-1/2 w-0.5 h-4 bg-dark-600" />
                  
                  <Link to={`/devices/${node.id}`}>
                    <motion.div
                      whileHover={{ scale: 1.02 }}
                      className={`bg-gradient-to-br ${
                        node.is_online
                          ? 'from-success/20 to-success/10 border-success'
                          : 'from-dark-700 to-dark-800 border-dark-600'
                      } border-2 rounded-xl p-4 cursor-pointer group relative`}
                    >
                      <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                        <ExternalLink className="w-4 h-4 text-dark-400" />
                      </div>
                      <div className="flex items-center justify-between mb-3">
                        <div className="flex items-center gap-2">
                          <Activity className={`w-5 h-5 ${node.is_online ? 'text-success' : 'text-dark-500'}`} />
                          <div>
                            <div className="font-semibold text-dark-100">{node.name}</div>
                            <div className="text-xs text-dark-400">
                              Node {node.mesh_node_id || 'N/A'}
                            </div>
                          </div>
                        </div>
                        <div className={`text-xs ${node.is_online ? 'text-success' : 'text-danger'}`}>
                          {node.is_online ? '🟢' : '🔴'}
                        </div>
                      </div>

                      <div className="grid grid-cols-2 gap-2 mb-3 text-xs">
                        <div>
                          <div className="text-dark-400">Platform</div>
                          <div className="text-dark-300">{node.platform || 'N/A'}</div>
                        </div>
                        <div>
                          <div className="text-dark-400">Firmware</div>
                          <div className="text-dark-300">{node.firmware || 'N/A'}</div>
                        </div>
                      </div>
                    </motion.div>
                  </Link>

                  <div className="flex gap-2 mt-2">
                    <button
                      onClick={(e) => { e.preventDefault(); handleCommand(node.id, 'status'); }}
                      className="flex-1 px-3 py-1.5 bg-primary/20 hover:bg-primary/30 text-primary rounded-lg text-xs font-medium transition-colors flex items-center justify-center gap-1"
                    >
                      <RefreshCw className="w-3 h-3" />
                      Refresh
                    </button>
                    <button
                      onClick={(e) => {
                        e.preventDefault();
                        if (confirm(`Reboot node "${node.name}"?`)) {
                          handleCommand(node.id, 'reboot');
                        }
                      }}
                      className="flex-1 px-3 py-1.5 bg-danger/20 hover:bg-danger/30 text-danger rounded-lg text-xs font-medium transition-colors flex items-center justify-center gap-1"
                    >
                      <Power className="w-3 h-3" />
                      Reboot
                    </button>
                  </div>
                </motion.div>
              ))}
            </div>
          </div>
        ) : (
          <div className="text-center py-12 text-dark-400">
            <Wifi className="w-12 h-12 mx-auto mb-4 opacity-50" />
            <p>No mesh nodes connected</p>
            <p className="text-sm text-dark-500 mt-2">
              Connect ESP32/ESP8266 nodes to see them here
            </p>
          </div>
        )}
      </motion.div>
    </div>
  );
}


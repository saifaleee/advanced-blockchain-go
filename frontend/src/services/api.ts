import axios from 'axios';

const API_URL = 'http://localhost:8080/api';

// Create axios instance with detailed error handling
const axiosInstance = axios.create({
  baseURL: API_URL,
  timeout: 10000, // 10 second timeout
  headers: {
    'Content-Type': 'application/json',
    'Accept': 'application/json'
  }
});

// Add response interceptor for debugging
axiosInstance.interceptors.response.use(
  response => {
    return response;
  },
  error => {
    console.error('API Error Details:', {
      message: error.message,
      status: error.response?.status,
      statusText: error.response?.statusText,
      data: error.response?.data,
      url: error.config?.url
    });
    return Promise.reject(error);
  }
);

// Types that match our API response structure
export interface BlockchainData {
  shards: ShardData[];
  validators: ValidatorData[];
  metrics: MetricsData;
}

export interface ShardData {
  id: number;
  blocks: BlockData[];
  txPoolSize: number;
  stateSize: number;
}

export interface BlockData {
  hash: string;
  prevHash: string;
  timestamp: number;
  nonce: number;
  transactions: TransactionData[];
  shardId: number;
  height: number;
}

export interface TransactionData {
  id: string;
  type: number;
  data: string;
  fromShard: number;
  toShard: number;
  status: number;
}

export interface ValidatorData {
  id: string;
  reputation: number;
  isActive: boolean;
}

export interface MetricsData {
  totalBlocks: number;
  totalTransactions: number;
  totalCrossShardTxs: number;
}

export interface TelemetryData {
  networkLatency: Record<string, number>;
  processingTimes: Record<string, number>;
  resourceUsage: Record<string, number>;
  timestamp: number;
}

// API service functions
const ApiService = {
  // Get the entire blockchain data
  getBlockchainData: async (): Promise<BlockchainData> => {
    try {
      const response = await axiosInstance.get('/blockchain');
      return response.data;
    } catch (error) {
      console.error('Error fetching blockchain data:', error);
      throw error;
    }
  },

  // Get telemetry data
  getTelemetryData: async (): Promise<TelemetryData> => {
    try {
      const response = await axiosInstance.get('/telemetry');
      return response.data;
    } catch (error) {
      console.error('Error fetching telemetry data:', error);
      throw error;
    }
  },

  // Helper function to get a specific shard by ID
  getShardById: async (shardId: number): Promise<ShardData | undefined> => {
    try {
      const data = await ApiService.getBlockchainData();
      return data.shards.find(shard => shard.id === shardId);
    } catch (error) {
      console.error(`Error fetching shard ${shardId}:`, error);
      throw error;
    }
  },

  // Helper function to get a specific block by hash and shard ID
  getBlockByHash: async (shardId: number, blockHash: string): Promise<BlockData | undefined> => {
    try {
      const shard = await ApiService.getShardById(shardId);
      return shard?.blocks.find(block => block.hash === blockHash);
    } catch (error) {
      console.error(`Error fetching block ${blockHash}:`, error);
      throw error;
    }
  }
};

export default ApiService; 
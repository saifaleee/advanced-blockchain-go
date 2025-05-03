import React, { useState, useEffect } from 'react';
import { Card, Row, Col, Table, Alert, Button } from 'react-bootstrap';
import { Bar, Line } from 'react-chartjs-2';
import ApiService, { TelemetryData } from '../services/api';
import { Chart as ChartJS, CategoryScale, LinearScale, BarElement, LineElement, Title, Tooltip, Legend, PointElement } from 'chart.js';

// Register ChartJS components
ChartJS.register(CategoryScale, LinearScale, BarElement, LineElement, PointElement, Title, Tooltip, Legend);

const NetworkStats: React.FC = () => {
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [telemetryData, setTelemetryData] = useState<TelemetryData | null>(null);

  const loadData = async () => {
    try {
      setLoading(true);
      const data = await ApiService.getTelemetryData();
      setTelemetryData(data);
      setError(null);
    } catch (err) {
      console.error('Failed to load telemetry data', err);
      setError('Failed to load network telemetry data. Please check if the blockchain server is running.');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
    
    // Auto-refresh every 5 seconds
    const interval = setInterval(loadData, 5000);
    return () => clearInterval(interval);
  }, []);

  // Prepare chart data for network latency
  const getLatencyChartData = () => {
    if (!telemetryData) return null;

    const keys = Object.keys(telemetryData.networkLatency);
    const values = Object.values(telemetryData.networkLatency);

    return {
      labels: keys,
      datasets: [
        {
          label: 'Network Latency (ms)',
          data: values,
          backgroundColor: 'rgba(54, 162, 235, 0.6)',
          borderColor: 'rgba(54, 162, 235, 1)',
          borderWidth: 1,
        },
      ],
    };
  };

  // Prepare chart data for processing times
  const getProcessingTimeChartData = () => {
    if (!telemetryData) return null;

    const keys = Object.keys(telemetryData.processingTimes);
    const values = Object.values(telemetryData.processingTimes);

    return {
      labels: keys,
      datasets: [
        {
          label: 'Processing Time (ms)',
          data: values,
          backgroundColor: 'rgba(255, 159, 64, 0.6)',
          borderColor: 'rgba(255, 159, 64, 1)',
          borderWidth: 1,
        },
      ],
    };
  };

  // Prepare chart data for resource usage
  const getResourceUsageChartData = () => {
    if (!telemetryData) return null;

    const keys = Object.keys(telemetryData.resourceUsage);
    const values = Object.values(telemetryData.resourceUsage);

    return {
      labels: keys,
      datasets: [
        {
          label: 'Resource Usage',
          data: values,
          backgroundColor: 'rgba(75, 192, 192, 0.6)',
          borderColor: 'rgba(75, 192, 192, 1)',
          borderWidth: 1,
        },
      ],
    };
  };

  return (
    <div className="network-stats">
      <h1 className="mb-4">Network Statistics</h1>
      
      {loading && !telemetryData && <Alert variant="info">Loading network telemetry data...</Alert>}
      {error && <Alert variant="danger">{error}</Alert>}
      
      <div className="mb-3 text-end">
        <Button variant="primary" onClick={loadData}>
          Refresh Data
        </Button>
      </div>
      
      {telemetryData && (
        <>
          <Card className="mb-4">
            <Card.Body>
              <Card.Title>Network Latency</Card.Title>
              <div className="chart-container">
                {getLatencyChartData() && <Bar data={getLatencyChartData()!} />}
              </div>
            </Card.Body>
          </Card>
          
          <Row className="mb-4">
            <Col md={6}>
              <Card className="h-100">
                <Card.Body>
                  <Card.Title>Processing Times</Card.Title>
                  <div className="chart-container">
                    {getProcessingTimeChartData() && <Bar data={getProcessingTimeChartData()!} />}
                  </div>
                </Card.Body>
              </Card>
            </Col>
            <Col md={6}>
              <Card className="h-100">
                <Card.Body>
                  <Card.Title>Resource Usage</Card.Title>
                  <div className="chart-container">
                    {getResourceUsageChartData() && <Bar data={getResourceUsageChartData()!} />}
                  </div>
                </Card.Body>
              </Card>
            </Col>
          </Row>
          
          <Card>
            <Card.Header>Network Telemetry Summary</Card.Header>
            <Card.Body>
              <Row>
                <Col md={4}>
                  <h5>Network Latency</h5>
                  <Table striped bordered hover>
                    <thead>
                      <tr>
                        <th>Metric</th>
                        <th>Value (ms)</th>
                      </tr>
                    </thead>
                    <tbody>
                      {Object.entries(telemetryData.networkLatency).map(([key, value]) => (
                        <tr key={key}>
                          <td>{key}</td>
                          <td>{value.toFixed(2)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </Table>
                </Col>
                <Col md={4}>
                  <h5>Processing Times</h5>
                  <Table striped bordered hover>
                    <thead>
                      <tr>
                        <th>Operation</th>
                        <th>Time (ms)</th>
                      </tr>
                    </thead>
                    <tbody>
                      {Object.entries(telemetryData.processingTimes).map(([key, value]) => (
                        <tr key={key}>
                          <td>{key}</td>
                          <td>{value.toFixed(2)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </Table>
                </Col>
                <Col md={4}>
                  <h5>Resource Usage</h5>
                  <Table striped bordered hover>
                    <thead>
                      <tr>
                        <th>Resource</th>
                        <th>Value</th>
                      </tr>
                    </thead>
                    <tbody>
                      {Object.entries(telemetryData.resourceUsage).map(([key, value]) => (
                        <tr key={key}>
                          <td>{key}</td>
                          <td>{value.toFixed(2)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </Table>
                </Col>
              </Row>
              <div className="text-muted mt-3">
                Last updated: {new Date(telemetryData.timestamp * 1000).toLocaleString()}
              </div>
            </Card.Body>
          </Card>
        </>
      )}
    </div>
  );
};

export default NetworkStats; 
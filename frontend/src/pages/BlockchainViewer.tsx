import React, { useState, useEffect, useRef } from 'react';
import { Row, Col, Card, Button, Form, Alert } from 'react-bootstrap';
import { Link } from 'react-router-dom';
import * as d3 from 'd3';
import ApiService, { BlockchainData, BlockData } from '../services/api';

const BlockchainViewer: React.FC = () => {
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [blockchainData, setBlockchainData] = useState<BlockchainData | null>(null);
  const [selectedShard, setSelectedShard] = useState<number | null>(null);
  const svgRef = useRef<SVGSVGElement>(null);

  const loadData = async () => {
    try {
      setLoading(true);
      const data = await ApiService.getBlockchainData();
      setBlockchainData(data);
      if (data.shards.length > 0 && selectedShard === null) {
        setSelectedShard(data.shards[0].id);
      }
      setError(null);
    } catch (err) {
      console.error('Failed to load blockchain data', err);
      setError('Failed to load blockchain data. Please check if the blockchain server is running.');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
    // Set up auto-refresh every 10 seconds
    const interval = setInterval(loadData, 10000);
    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    if (blockchainData && selectedShard !== null && svgRef.current) {
      renderBlockchain();
    }
  }, [blockchainData, selectedShard]);

  const renderBlockchain = () => {
    if (!blockchainData || selectedShard === null || !svgRef.current) return;

    // Find the selected shard
    const shard = blockchainData.shards.find(s => s.id === selectedShard);
    if (!shard) return;

    // Clear previous visualization
    d3.select(svgRef.current).selectAll('*').remove();

    const width = 960;
    const height = 300;
    const blockWidth = 120;
    const blockHeight = 80;
    const blockMargin = 40;
    const svg = d3.select(svgRef.current)
        .attr('width', width)
        .attr('height', height);

    // Create a group for the visualization
    const g = svg.append('g')
        .attr('transform', `translate(${blockMargin}, ${height / 2 - blockHeight / 2})`);

    // Sort blocks by height
    const blocks = [...shard.blocks].sort((a, b) => a.height - b.height);

    // Create nodes for each block
    blocks.forEach((block, i) => {
      const blockGroup = g.append('g')
        .attr('transform', `translate(${i * (blockWidth + blockMargin)}, 0)`)
        .attr('class', 'block')
        .on('click', () => {
          window.location.href = `/block/${selectedShard}/${block.hash}`;
        })
        .style('cursor', 'pointer');

      // Block rectangle
      blockGroup.append('rect')
        .attr('width', blockWidth)
        .attr('height', blockHeight)
        .attr('rx', 5)
        .attr('ry', 5)
        .attr('fill', '#f8f9fa')
        .attr('stroke', '#6c757d')
        .attr('stroke-width', 2);

      // Block height
      blockGroup.append('text')
        .attr('x', blockWidth / 2)
        .attr('y', 20)
        .attr('text-anchor', 'middle')
        .attr('fill', '#212529')
        .style('font-weight', 'bold')
        .text(`Block ${block.height}`);

      // Block hash (shortened)
      blockGroup.append('text')
        .attr('x', blockWidth / 2)
        .attr('y', 40)
        .attr('text-anchor', 'middle')
        .attr('fill', '#495057')
        .style('font-size', '10px')
        .text(`${block.hash.substring(0, 10)}...`);

      // Transaction count
      blockGroup.append('text')
        .attr('x', blockWidth / 2)
        .attr('y', 60)
        .attr('text-anchor', 'middle')
        .attr('fill', '#6c757d')
        .text(`${block.transactions.length} tx`);

      // Add connection lines between blocks
      if (i > 0) {
        g.append('line')
          .attr('x1', (i - 1) * (blockWidth + blockMargin) + blockWidth)
          .attr('y1', blockHeight / 2)
          .attr('x2', i * (blockWidth + blockMargin))
          .attr('y2', blockHeight / 2)
          .attr('stroke', '#adb5bd')
          .attr('stroke-width', 2)
          .attr('marker-end', 'url(#arrow)');
      }
    });

    // Add arrow marker for the lines
    svg.append('defs').append('marker')
      .attr('id', 'arrow')
      .attr('viewBox', '0 -5 10 10')
      .attr('refX', 8)
      .attr('refY', 0)
      .attr('markerWidth', 6)
      .attr('markerHeight', 6)
      .attr('orient', 'auto')
      .append('path')
      .attr('d', 'M0,-5L10,0L0,5')
      .attr('fill', '#adb5bd');
  };

  return (
    <div className="blockchain-viewer">
      <h1 className="mb-4">Blockchain Viewer</h1>
      
      {loading && !blockchainData && <Alert variant="info">Loading blockchain data...</Alert>}
      {error && <Alert variant="danger">{error}</Alert>}
      
      {blockchainData && (
        <>
          <Card className="mb-4">
            <Card.Body>
              <Row>
                <Col md={4}>
                  <Form.Group>
                    <Form.Label>Select Shard</Form.Label>
                    <Form.Select 
                      value={selectedShard || ''}
                      onChange={(e) => setSelectedShard(Number(e.target.value))}
                    >
                      {blockchainData.shards.map(shard => (
                        <option key={shard.id} value={shard.id}>
                          Shard {shard.id} ({shard.blocks.length} blocks)
                        </option>
                      ))}
                    </Form.Select>
                  </Form.Group>
                </Col>
                <Col md={4} className="d-flex align-items-end">
                  <Button variant="primary" onClick={loadData}>
                    Refresh Data
                  </Button>
                </Col>
              </Row>
            </Card.Body>
          </Card>

          <Card className="mb-4">
            <Card.Body>
              <Card.Title>Visual Blockchain</Card.Title>
              <div className="blockchain-visualization">
                <svg ref={svgRef} style={{ maxWidth: '100%', overflow: 'auto' }}></svg>
              </div>
            </Card.Body>
          </Card>

          {selectedShard !== null && (
            <Card>
              <Card.Header>Blocks in Shard {selectedShard}</Card.Header>
              <Card.Body>
                <div className="block-list" style={{ maxHeight: '500px', overflowY: 'auto' }}>
                  <Row>
                    {blockchainData.shards
                      .find(shard => shard.id === selectedShard)
                      ?.blocks
                      .sort((a, b) => b.height - a.height)
                      .map(block => (
                        <Col md={4} key={block.hash} className="mb-3">
                          <Card>
                            <Card.Body>
                              <Card.Title>Block {block.height}</Card.Title>
                              <Card.Text>
                                <strong>Hash:</strong> {block.hash.substring(0, 16)}...<br />
                                <strong>Prev Hash:</strong> {block.prevHash.substring(0, 16)}...<br />
                                <strong>Transactions:</strong> {block.transactions.length}<br />
                                <strong>Timestamp:</strong> {new Date(block.timestamp * 1000).toLocaleString()}<br />
                              </Card.Text>
                              <Link to={`/block/${selectedShard}/${block.hash}`} className="btn btn-sm btn-primary">
                                View Details
                              </Link>
                            </Card.Body>
                          </Card>
                        </Col>
                      ))
                    }
                  </Row>
                </div>
              </Card.Body>
            </Card>
          )}
        </>
      )}
    </div>
  );
};

export default BlockchainViewer; 
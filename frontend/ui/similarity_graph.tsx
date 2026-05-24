import * as React from 'react';
import {
  forceSimulation,
  forceLink,
  forceManyBody,
  forceCenter,
  forceCollide,
} from 'd3-force';

const TEAM_COLORS = {
  red: '#d13030',
  blue: '#4183cc',
  neutral: '#b8a88a',
  black: '#222',
};

const DARK_TEAM_COLORS = {
  red: '#e05050',
  blue: '#5a9de0',
  neutral: '#c4b896',
  black: '#888',
};

interface SimilarityGraphProps {
  words: string[];
  layout: string[];
  revealed: boolean[];
  similarity: number[][];
  cluegiver: boolean;
  winningTeam: string | null;
  darkMode: boolean;
}

interface GraphNode {
  index: number;
  word: string;
  team: string;
  revealed: boolean;
  x?: number;
  y?: number;
  fx?: number | null;
  fy?: number | null;
}

interface GraphLink {
  source: number;
  target: number;
  similarity: number;
}

interface SimilarityGraphState {
  nodes: GraphNode[];
  links: GraphLink[];
  dragNode: number | null;
}

export class SimilarityGraph extends React.Component<
  SimilarityGraphProps,
  SimilarityGraphState
> {
  private simulation: any;
  private svgRef: React.RefObject<SVGSVGElement>;
  private width = 700;
  private height = 500;

  constructor(props: SimilarityGraphProps) {
    super(props);
    this.svgRef = React.createRef();

    const { nodes, links } = this.buildGraph(props);
    this.state = { nodes, links, dragNode: null };
  }

  private buildGraph(props: SimilarityGraphProps) {
    const blackIdx = props.layout.indexOf('black');
    const nodes: GraphNode[] = props.words.map((word, i) => {
      const node: GraphNode = {
        index: i,
        word,
        team: props.layout[i],
        revealed: props.revealed[i],
      };
      if (i === blackIdx) {
        node.x = this.width / 2;
        node.y = 40;
      } else {
        const sim = props.similarity && blackIdx >= 0
          ? props.similarity[blackIdx][i] || 0
          : 0;
        const angle = -Math.PI / 2 + (Math.random() - 0.5) * Math.PI * 1.4;
        const dist = 80 + 300 * (1 - sim);
        node.x = this.width / 2 + Math.cos(angle) * dist;
        node.y = 40 + Math.abs(Math.sin(angle)) * dist + 40;
      }
      return node;
    });

    const links: GraphLink[] = [];
    if (props.similarity) {
      for (let i = 0; i < props.words.length; i++) {
        for (let j = i + 1; j < props.words.length; j++) {
          const sim = props.similarity[i][j];
          if (sim > 0) {
            links.push({ source: i, target: j, similarity: sim });
          }
        }
      }
    }

    return { nodes, links };
  }

  private showTeamColor(idx: number): boolean {
    return (
      this.props.cluegiver ||
      this.props.revealed[idx] ||
      !!this.props.winningTeam
    );
  }

  private nodeColor(node: GraphNode): string {
    const colors = this.props.darkMode ? DARK_TEAM_COLORS : TEAM_COLORS;
    if (this.showTeamColor(node.index)) {
      return colors[node.team] || colors.neutral;
    }
    return this.props.darkMode ? '#757575' : '#e8e8e8';
  }

  private nodeStroke(node: GraphNode): string {
    if (this.showTeamColor(node.index) && node.team === 'black') {
      return this.props.darkMode ? '#fff' : '#000';
    }
    return this.props.darkMode ? '#555' : '#ccc';
  }

  private textColor(node: GraphNode): string {
    if (this.showTeamColor(node.index)) {
      if (node.team === 'black' || node.team === 'red' || node.team === 'blue') {
        return '#fff';
      }
    }
    return this.props.darkMode ? '#eee' : '#333';
  }

  componentDidMount() {
    this.startSimulation();
  }

  componentWillUnmount() {
    if (this.simulation) {
      this.simulation.stop();
    }
  }

  componentDidUpdate(prevProps: SimilarityGraphProps) {
    if (
      prevProps.words !== this.props.words ||
      prevProps.similarity !== this.props.similarity
    ) {
      if (this.simulation) this.simulation.stop();
      const { nodes, links } = this.buildGraph(this.props);
      this.setState({ nodes, links }, () => this.startSimulation());
    }
  }

  private startSimulation() {
    const { nodes, links } = this.state;
    if (!links.length) return;

    const maxSim = Math.max(...links.map((l) => l.similarity), 0.01);

    this.simulation = forceSimulation(nodes as any)
      .force(
        'link',
        forceLink(links as any)
          .id((d: any) => d.index)
          .distance((d: any) => {
            return 50 + 200 * (1 - d.similarity / maxSim);
          })
          .strength((d: any) => {
            return 0.1 + 0.4 * (d.similarity / maxSim);
          })
      )
      .force('charge', forceManyBody().strength(-120))
      .force('center', forceCenter(this.width / 2, this.height / 2))
      .force('collide', forceCollide(28))
      .on('tick', () => {
        this.forceUpdate();
      });
  }

  private handleMouseDown = (e: React.MouseEvent, idx: number) => {
    e.preventDefault();
    const node = this.state.nodes[idx] as any;
    node.fx = node.x;
    node.fy = node.y;
    this.setState({ dragNode: idx });

    const onMove = (me: MouseEvent) => {
      const svg = this.svgRef.current;
      if (!svg) return;
      const rect = svg.getBoundingClientRect();
      node.fx = me.clientX - rect.left;
      node.fy = me.clientY - rect.top;
      if (this.simulation) this.simulation.alpha(0.3).restart();
    };

    const onUp = () => {
      node.fx = null;
      node.fy = null;
      this.setState({ dragNode: null });
      if (this.simulation) this.simulation.alpha(0.3).restart();
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
    };

    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
  };

  render() {
    const { nodes, links } = this.state;

    if (!this.props.similarity || !this.props.similarity.length) {
      return (
        <div className="similarity-graph-empty">
          Similarity data not available for this game.
        </div>
      );
    }

    const maxSim = Math.max(...links.map((l) => l.similarity), 0.01);

    return (
      <svg
        ref={this.svgRef}
        className="similarity-graph"
        width={this.width}
        height={this.height}
        style={{ border: '1px solid ' + (this.props.darkMode ? '#555' : '#ddd'), borderRadius: 8 }}
      >
        {links.map((link, i) => {
          const s = link.source as unknown as GraphNode;
          const t = link.target as unknown as GraphNode;
          if (!s || !t || s.x == null || t.x == null) return null;
          const opacity = 0.03 + 0.6 * (link.similarity / maxSim);
          const width = 0.5 + 2 * (link.similarity / maxSim);
          return (
            <line
              key={i}
              x1={s.x}
              y1={s.y}
              x2={t.x}
              y2={t.y}
              stroke={this.props.darkMode ? '#aaa' : '#999'}
              strokeWidth={width}
              strokeOpacity={opacity}
            />
          );
        })}
        {nodes.map((node, i) => {
          if (node.x == null || node.y == null) return null;
          return (
            <g
              key={i}
              transform={`translate(${node.x},${node.y})`}
              onMouseDown={(e) => this.handleMouseDown(e, i)}
              style={{ cursor: 'grab' }}
            >
              <circle
                r={18}
                fill={this.nodeColor(node)}
                stroke={this.nodeStroke(node)}
                strokeWidth={node.team === 'black' && this.showTeamColor(i) ? 3 : 1.5}
              />
              <text
                textAnchor="middle"
                dy="0.35em"
                fontSize={node.word.length > 7 ? 7 : 8}
                fontWeight="bold"
                fill={this.textColor(node)}
                pointerEvents="none"
              >
                {node.word}
              </text>
            </g>
          );
        })}
      </svg>
    );
  }
}

#!/usr/bin/env python3
import json
import graphviz
import re
import sys
from pathlib import Path

def parse_waveplan_json(json_path):
    """
    Extract units and dependency edges from Waveplan Execution Waves JSON.
    
    Returns:
        units: dict {unit_id: {title, kind, wave, depends_on, ...}}
        edges: list of (src_id, dst_id) tuples
        waves: dict {wave_num: [unit_ids]} (optional grouping)
    """
    with open(json_path, 'r') as f:
        data = json.load(f)
    
    units = data.get('units', {})
    edges = []
    
    # Extract dependency edges: depends_on[unit] → unit
    for unit_id, unit_data in units.items():
        depends = unit_data.get('depends_on', [])
        for dep_id in depends:
            if dep_id in units:  # Only include valid references
                edges.append((dep_id, unit_id))
    
    # Optional: extract wave groupings for coloring/annotation
    waves = {}
    for wave_def in data.get('waves', []):
        wave_num = wave_def['wave']
        waves[wave_num] = wave_def.get('units', [])
    
    return units, edges, waves

def get_kind_color(kind):
    """Map unit kind to subtle fill color variation."""
    palette = {
        'impl': '#E6F2FF',      # powder-blue (default)
        'test': '#E8F5E9',      # soft green
        'verify': '#FFF3E0',    # soft orange
        'doc': '#F3E5F5',       # soft purple
        'refactor': '#ECEFF1',  # soft gray
    }
    return palette.get(kind, '#E6F2FF')

def render_waveplan_dag(units, edges, waves, output_path, title="Waveplan Execution DAG"):
    """Render DAG with orthogonal edges, solid boxes, vertical layout, wave-aware styling."""
    
    dot = graphviz.Digraph(
        comment=title,
        format='png',  # or 'svg'
        graph_attr={
            'rankdir': 'TB',           # Top-to-Bottom vertical layout
            'splines': 'ortho',        # Orthogonal (angled) edges
            'nodesep': '0.5',          # Horizontal spacing
            'ranksep': '0.9',          # Vertical spacing (more room for labels)
            'fontname': 'Helvetica',
            'fontsize': '16',
            'pad': '0.5',
            'label': title,
            'labelloc': 't',
            'labeljust': 'l',
        },
        node_attr={
            'style': 'filled,solid',   # No transparency
            'fillcolor': '#E6F2FF',    # Default powder-blue
            'color': '#4A6FA5',        # Border color (slate-blue)
            'penwidth': '4',           # 4px border width
            'fontname': 'Helvetica',
            'fontsize': '17',          # ~17pt equivalent
            'fontcolor': '#1A1A1A',
            'shape': 'rect',           # Clean rectangular boxes
            'margin': '0.3,0.2',
            'width': '1.3',
            'height': '0.7',
        },
        edge_attr={
            'color': '#666666',
            'penwidth': '2',
            'arrowsize': '0.8',
            'arrowhead': 'vee',
        }
    )
    
    # Add nodes with kind-based coloring and wave annotation
    for unit_id, unit_data in units.items():
        title = unit_data.get('title', unit_id)
        kind = unit_data.get('kind', 'impl')
        wave = unit_data.get('wave', None)
        
        # Sanitize label for Graphviz
        clean_title = title.replace('"', '\\"')
        if len(clean_title) > 50:
            clean_title = clean_title[:47] + "…"
        
        # Apply text contractor to title
        import sys
        import os
        import json
        sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
        from title_contractor import MiddleOutTitleContractor
        config_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'contr_conf.json')
        config = {}
        if os.path.exists(config_path):
            with open(config_path, 'r') as f:
                config = json.load(f)
        extra_words = config.get('word_map', {})
        extra_bigrams = config.get('bigram_map', {})
        if not hasattr(render_waveplan_dag, '_contractor'):
            render_waveplan_dag._contractor = MiddleOutTitleContractor(
                contract_words=True,
                contract_bigrams=True
            )
        contracted, _ = render_waveplan_dag._contractor.contract(
            clean_title,
            extra_word_map=extra_words,
            extra_bigram_map=extra_bigrams
        )
        label = f"W{wave}-{unit_id}\\n{contracted}"
        
        # Apply kind-based fill color
        fillcolor = get_kind_color(kind)
        
        dot.node(
            unit_id,
            label=label,
            fillcolor=fillcolor,
            tooltip=f"{unit_id} • {kind}" + (f" • Wave {wave}" if wave else "")
        )
    
    # Add edges
    for src, dst in edges:
        dot.edge(src, dst)
    
    # Optional: Add subtle wave separation using subgraphs (for visual grouping)
    if waves:
        for wave_num in sorted(waves.keys()):
            wave_units = [u for u in waves[wave_num] if u in units]
            if len(wave_units) >= 2:
                with dot.subgraph(name=f'cluster_wave_{wave_num}') as sub:
                    sub.attr(
                        label=f'Wave {wave_num}',
                        style='dashed',
                        color='#AAAAAA',
                        fontname='Helvetica',
                        fontsize='12',
                        fontcolor='#666666',
                        margin='20'
                    )
                    for uid in wave_units:
                        sub.node(uid)  # Re-declare to include in subgraph
    
    # Render
    output = dot.render(output_path, cleanup=True)
    print(f"✓ Rendered to: {output}")
    print(f"  • {len(units)} units, {len(edges)} dependencies")
    if waves:
        print(f"  • {len(waves)} wave groups")
    return output

if __name__ == "__main__":
    if len(sys.argv) < 3:
        print("Usage: python render_waveplan_dag.py <input.json> <output_basename>")
        print("Example: python render_waveplan_dag.py plan.json output/waveplan_viz")
        sys.exit(1)
    
    json_file = Path(sys.argv[1])
    output_base = sys.argv[2]
    
    if not json_file.exists():
        print(f"✗ File not found: {json_file}")
        sys.exit(1)
    
    units, edges, waves = parse_waveplan_json(json_file)
    
    if not units:
        print("⚠ No units found in input file")
        sys.exit(0)
    
    render_waveplan_dag(units, edges, waves, output_base, title=f"Waveplan: {json_file.stem}")


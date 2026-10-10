CREATE TABLE entity_graph_nodes (
    id bigserial PRIMARY KEY,
    entity_hint text NOT NULL,
    node_type text NOT NULL,
    display_label text NOT NULL DEFAULT '',
    public_risk_tags text[] NOT NULL DEFAULT '{}'::text[],
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (entity_hint, node_type),
    CHECK (btrim(entity_hint) <> ''),
    CHECK (node_type IN ('customer_entity', 'counterparty', 'case_cluster', 'edd_status'))
);

CREATE TABLE entity_graph_edges (
    id bigserial PRIMARY KEY,
    source_node_id bigint NOT NULL REFERENCES entity_graph_nodes(id) ON DELETE CASCADE,
    target_node_id bigint NOT NULL REFERENCES entity_graph_nodes(id) ON DELETE CASCADE,
    relation_type text NOT NULL,
    strength integer NOT NULL DEFAULT 0,
    public_summary text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source_node_id, target_node_id, relation_type),
    CHECK (source_node_id <> target_node_id),
    CHECK (relation_type IN ('shared_entity_hint', 'shared_counterparty', 'same_case_cluster', 'edd_status_overlap')),
    CHECK (strength >= 0 AND strength <= 100)
);

CREATE INDEX entity_graph_nodes_entity_hint_idx ON entity_graph_nodes(entity_hint);
CREATE INDEX entity_graph_nodes_node_type_idx ON entity_graph_nodes(node_type);
CREATE INDEX entity_graph_edges_source_node_id_idx ON entity_graph_edges(source_node_id);
CREATE INDEX entity_graph_edges_target_node_id_idx ON entity_graph_edges(target_node_id);
CREATE INDEX entity_graph_edges_relation_type_idx ON entity_graph_edges(relation_type);

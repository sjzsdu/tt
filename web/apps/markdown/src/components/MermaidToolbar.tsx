import { Button, Slider, Space, Tooltip } from 'antd';
import {
  CopyOutlined,
  ExpandOutlined,
  FileTextOutlined,
  FullscreenExitOutlined,
  ZoomInOutlined,
  ZoomOutOutlined,
} from '@ant-design/icons';

interface MermaidToolbarProps {
  scale: number;
  onZoomIn: () => void;
  onZoomOut: () => void;
  onReset: () => void;
  onScaleChange: (scale: number) => void;
  onExportSvg: () => void;
  onExportPng: () => void;
  onCopy: () => void;
}

export function MermaidToolbar({
  scale,
  onZoomIn,
  onZoomOut,
  onReset,
  onScaleChange,
  onExportSvg,
  onExportPng,
  onCopy,
}: MermaidToolbarProps) {
  return (
    <div className="mermaid-toolbar">
      <Slider
        min={40}
        max={400}
        value={Math.round(scale * 100)}
        tooltip={{ formatter: v => `${v}%` }}
        style={{ width: 100 }}
        onChange={v => onScaleChange(v / 100)}
      />
      <div className="mermaid-toolbar-actions">
        <Space>
          <Tooltip title="Zoom Out">
            <Button size="small" icon={<ZoomOutOutlined />} onClick={onZoomOut} />
          </Tooltip>
          <Tooltip title="Reset">
            <Button size="small" icon={<FullscreenExitOutlined />} onClick={onReset} />
          </Tooltip>
          <Tooltip title="Zoom In">
            <Button size="small" icon={<ZoomInOutlined />} onClick={onZoomIn} />
          </Tooltip>
          <Tooltip title="Export SVG">
            <Button size="small" icon={<FileTextOutlined />} onClick={onExportSvg} />
          </Tooltip>
          <Tooltip title="Export PNG">
            <Button size="small" icon={<ExpandOutlined />} onClick={onExportPng} />
          </Tooltip>
          <Tooltip title="Copy">
            <Button size="small" icon={<CopyOutlined />} onClick={onCopy} />
          </Tooltip>
        </Space>
      </div>
    </div>
  );
}

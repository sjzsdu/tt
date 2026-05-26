import { Button, Slider, Space, Tooltip } from 'antd';
import {
  CopyOutlined,
  DownloadOutlined,
  FileTextOutlined,
  FullscreenExitOutlined,
  ZoomInOutlined,
  ZoomOutOutlined,
} from '@ant-design/icons';

interface MermaidToolbarProps {
  scale: number;
  title: string;
  subtitle: string;
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
  title,
  subtitle,
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
      <div className="mermaid-toolbar-title">
        <strong>{title}</strong>
        <span>{subtitle}</span>
      </div>
      <div className="mermaid-toolbar-actions">
        <Slider
          min={25}
          max={400}
          value={Math.round(scale * 100)}
          tooltip={{ formatter: v => `${v}%` }}
          className="mermaid-zoom-slider"
          onChange={v => onScaleChange(v / 100)}
        />
        <Space size={6} wrap>
          <Tooltip title="Zoom Out">
            <Button size="small" icon={<ZoomOutOutlined />} onClick={onZoomOut} />
          </Tooltip>
          <Tooltip title="Fit to view">
            <Button size="small" icon={<FullscreenExitOutlined />} onClick={onReset} />
          </Tooltip>
          <Tooltip title="Zoom In">
            <Button size="small" icon={<ZoomInOutlined />} onClick={onZoomIn} />
          </Tooltip>
          <Tooltip title="Export SVG">
            <Button size="small" icon={<FileTextOutlined />} onClick={onExportSvg} />
          </Tooltip>
          <Tooltip title="Export PNG">
            <Button size="small" icon={<DownloadOutlined />} onClick={onExportPng} />
          </Tooltip>
          <Tooltip title="Copy">
            <Button size="small" icon={<CopyOutlined />} onClick={onCopy} />
          </Tooltip>
        </Space>
      </div>
    </div>
  );
}

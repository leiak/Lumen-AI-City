/**
 * NPC 显示名查找表（Sprint 12 T03d min slice）。
 *
 * 为什么单独文件：
 *   - backend（world-engine Tile.npc_ids）已提供"哪些 NPC 在哪些 tile"，
 *     无需在这重复位置信息；这里只放 display name 等"富描述"属性
 *     （backend 1.0 不暴露 NPC 元数据 endpoint）
 *   - 单点真相：WorldMap 渲染 NPC 标签、NPCDialog 显示角色名都从这里读
 *     —— 避免 NPCDialog.tsx 内嵌的 NPC_NAMES 与此处分叉
 *
 * 1.0 维护方式：
 *   - 与 packages/npc-templates/*.yaml 的 npc_id 同步（手维护）
 *   - 2.0 替换为 /v1/npcs/:id GET 返回值（含 kind / portrait_url / talk_tree_ref 等）
 */

/** npc_id → 显示名（中文）。fallback 到 npc_id 自身，避免 UI 显示空白 */
const NPC_DISPLAY_NAMES: Record<string, string> = {
  npc_wang_boss_001: '王老板',
  npc_lihua_001: '李华',
};

/** 查 NPC 显示名。找不到时返回 npc_id 自身（Sprint 12 NPCDialog 的同款兜底） */
export function getNpcDisplayName(npcId: string): string {
  return NPC_DISPLAY_NAMES[npcId] ?? npcId;
}

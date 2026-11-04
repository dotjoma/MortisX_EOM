export const ICONS = [
  'smart_toy',
  'format_list_bulleted',
  'code',
  'database',
  'settings',
  'person',
  'map',
  'backpack',
  'storefront',
  'terminal',
  'bolt',
  'receipt_long',
  'open_with',
  'history',
  'swords',
  'arrow_upward',
  'arrow_downward',
  'arrow_back',
  'arrow_forward',
  'door_open',
  'link',
  'link_off',
  'logout',
  'near_me',
  'add',
  'download',
  'save',
  'search',
  'chevron_left',
  'chevron_right',
  'delete',
  'play_arrow',
  'stop',
  'close',
  'minimize',
  'crop_square',
  'check_circle',
  'warning',
  'cancel',
  'info',
  // Extra icons audited & verified in local TTF font
  'lock',
  'favorite',
  'radar',
  'route',
  'open_in_new',
  'star',
  'apps',
  'mail',
  'inbox',
  'edit',
  'reply',
  'assignment',
  'pets',
  'card_giftcard',
  'redeem',
  'casino',
  'account_balance',
  'refresh',
  'emoji_events',
  'menu_book',
  'auto_fix_high',
  'group',
  'key',
  'military_tech',
  'favorite_border',
] as const;

export type IconName = (typeof ICONS)[number];
export type IconSize = 12 | 14 | 16 | 18 | 20 | 24 | 32 | 40 | 48;

export function Icon({
  name,
  size = 18,
  label,
  className,
}: {
  name: IconName;
  size?: IconSize;
  label?: string;
  className?: string;
}) {
  return (
    <span
      className={`icon ${className ?? ''}`}
      style={{ fontSize: size }}
      aria-hidden={label ? undefined : true}
      aria-label={label}
      role={label ? 'img' : undefined}
    >
      {name}
    </span>
  );
}

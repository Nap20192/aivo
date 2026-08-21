/* @ds-bundle: {"format":4,"namespace":"AIVODesignSystem_3d538f","components":[{"name":"Badge","sourcePath":"components/core/Badge.jsx"},{"name":"Button","sourcePath":"components/core/Button.jsx"},{"name":"Card","sourcePath":"components/core/Card.jsx"},{"name":"Icon","sourcePath":"components/core/Icon.jsx"},{"name":"IconButton","sourcePath":"components/core/IconButton.jsx"},{"name":"StatCard","sourcePath":"components/core/StatCard.jsx"},{"name":"DataTable","sourcePath":"components/data/DataTable.jsx"},{"name":"KeyValueList","sourcePath":"components/data/KeyValueList.jsx"},{"name":"MoneyAmount","sourcePath":"components/data/MoneyAmount.jsx"},{"name":"StatusPill","sourcePath":"components/data/StatusPill.jsx"},{"name":"AIInsight","sourcePath":"components/feedback/AIInsight.jsx"},{"name":"Dialog","sourcePath":"components/feedback/Dialog.jsx"},{"name":"EmptyState","sourcePath":"components/feedback/EmptyState.jsx"},{"name":"Toast","sourcePath":"components/feedback/Toast.jsx"},{"name":"Tooltip","sourcePath":"components/feedback/Tooltip.jsx"},{"name":"Checkbox","sourcePath":"components/forms/Checkbox.jsx"},{"name":"Input","sourcePath":"components/forms/Input.jsx"},{"name":"QuantityStepper","sourcePath":"components/forms/QuantityStepper.jsx"},{"name":"Select","sourcePath":"components/forms/Select.jsx"},{"name":"Switch","sourcePath":"components/forms/Switch.jsx"},{"name":"SidebarNav","sourcePath":"components/navigation/SidebarNav.jsx"},{"name":"Tabs","sourcePath":"components/navigation/Tabs.jsx"},{"name":"TopBar","sourcePath":"components/navigation/TopBar.jsx"}],"sourceHashes":{"components/core/Badge.jsx":"939d8caa4332","components/core/Button.jsx":"f70126fe77ce","components/core/Card.jsx":"cdb776f56ef9","components/core/Icon.jsx":"97df1e50a296","components/core/IconButton.jsx":"4b7766542c79","components/core/StatCard.jsx":"603fbe755fb5","components/data/DataTable.jsx":"51648f5a14c9","components/data/KeyValueList.jsx":"50fed26c19b9","components/data/MoneyAmount.jsx":"7079e6f40ab0","components/data/StatusPill.jsx":"3ff243c127b0","components/feedback/AIInsight.jsx":"b41683c95d4b","components/feedback/Dialog.jsx":"edd7ecfe4df2","components/feedback/EmptyState.jsx":"17644d130a0d","components/feedback/Toast.jsx":"e94dc62a5d7c","components/feedback/Tooltip.jsx":"48dcd456a1dc","components/forms/Checkbox.jsx":"0770d7a6d5b1","components/forms/Input.jsx":"e97db868c6be","components/forms/QuantityStepper.jsx":"01dbfc17812e","components/forms/Select.jsx":"2ce2c6730122","components/forms/Switch.jsx":"9dc254cf3a8a","components/navigation/SidebarNav.jsx":"df254e9cdd7c","components/navigation/Tabs.jsx":"5c7162dde197","components/navigation/TopBar.jsx":"58f544809429","ui_kits/backoffice/MenuScreen.jsx":"90accf8773bf","ui_kits/backoffice/ShiftReviewScreen.jsx":"8bf9f67f3eb8","ui_kits/backoffice/TodayScreen.jsx":"9cb8a868ac7a","ui_kits/backoffice/data.js":"0f81310b3fe9","ui_kits/pos/CloseShiftScreen.jsx":"9b03faa88f3e","ui_kits/pos/OpenShiftScreen.jsx":"c29ea83b92c8","ui_kits/pos/OrderScreen.jsx":"5df2dd31ff08","ui_kits/pos/data.js":"7455e2259960"},"inlinedExternals":[],"unexposedExports":[]} */

(() => {

const __ds_ns = (window.AIVODesignSystem_3d538f = window.AIVODesignSystem_3d538f || {});

const __ds_scope = {};

(__ds_ns.__errors = __ds_ns.__errors || []);

// components/core/Card.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/** Paper panel: hairline border, near-flat, warm white. */
function Card({
  title,
  eyebrow,
  actions,
  footer,
  padding = "var(--pad-card)",
  tone = "default",
  children,
  style,
  ...rest
}) {
  const tones = {
    default: {
      background: "var(--surface-card)",
      border: "1px solid var(--border-default)"
    },
    sunken: {
      background: "var(--surface-sunken)",
      border: "1px solid var(--border-subtle)"
    },
    accent: {
      background: "var(--surface-card)",
      border: "1px solid var(--red-200)",
      boxShadow: "0 0 0 3px var(--red-50)"
    },
    inverse: {
      background: "var(--surface-inverse)",
      border: "1px solid var(--ink-800)",
      color: "var(--text-inverse)"
    }
  };
  const header = title || eyebrow || actions;
  return /*#__PURE__*/React.createElement("section", _extends({
    style: {
      borderRadius: "var(--radius-md)",
      boxShadow: "var(--shadow-sm)",
      overflow: "hidden",
      ...tones[tone],
      ...style
    }
  }, rest), header ? /*#__PURE__*/React.createElement("header", {
    style: {
      display: "flex",
      alignItems: "center",
      justifyContent: "space-between",
      gap: "var(--space-5)",
      padding: "var(--space-5) " + "var(--pad-card)",
      borderBottom: "1px solid var(--border-subtle)"
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gap: 2
    }
  }, eyebrow ? /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--type-eyebrow)",
      letterSpacing: "var(--tracking-caps)",
      textTransform: "uppercase",
      color: tone === "inverse" ? "var(--ink-300)" : "var(--text-muted)"
    }
  }, eyebrow) : null, title ? /*#__PURE__*/React.createElement("h3", {
    style: {
      font: "var(--type-section-title)",
      color: tone === "inverse" ? "var(--paper-1)" : "var(--text-strong)"
    }
  }, title) : null), actions ? /*#__PURE__*/React.createElement("div", {
    style: {
      display: "flex",
      alignItems: "center",
      gap: "var(--gap-inline)"
    }
  }, actions) : null) : null, /*#__PURE__*/React.createElement("div", {
    style: {
      padding
    }
  }, children), footer ? /*#__PURE__*/React.createElement("footer", {
    style: {
      padding: "var(--space-5) var(--pad-card)",
      borderTop: "1px solid var(--border-subtle)",
      background: tone === "inverse" ? "transparent" : "var(--paper-1)"
    }
  }, footer) : null);
}
Object.assign(__ds_scope, { Card });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/Card.jsx", error: String((e && e.message) || e) }); }

// components/core/Icon.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const CDN = "https://unpkg.com/lucide-static@0.469.0/icons/";
const cache = {};

/** Lucide glyph inlined as SVG so it inherits currentColor. */
function Icon({
  name,
  size = 18,
  color,
  style,
  title,
  ...rest
}) {
  const [svg, setSvg] = React.useState(cache[name] || null);
  React.useEffect(() => {
    if (cache[name]) {
      setSvg(cache[name]);
      return;
    }
    let live = true;
    fetch(CDN + name + ".svg").then(r => r.ok ? r.text() : "").then(t => {
      cache[name] = t;
      if (live) setSvg(t);
    }).catch(() => {});
    return () => {
      live = false;
    };
  }, [name]);
  return /*#__PURE__*/React.createElement("span", _extends({
    role: title ? "img" : "presentation",
    "aria-label": title,
    "aria-hidden": title ? undefined : true,
    dangerouslySetInnerHTML: svg ? {
      __html: svg.replace("<svg", '<svg width="100%" height="100%"')
    } : undefined,
    style: {
      display: "inline-flex",
      alignItems: "center",
      justifyContent: "center",
      flex: "none",
      width: size,
      height: size,
      color: color || "currentColor",
      ...style
    }
  }, rest));
}
Object.assign(__ds_scope, { Icon });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/Icon.jsx", error: String((e && e.message) || e) }); }

// components/core/Badge.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const tones = {
  neutral: {
    color: "var(--ink-700)",
    background: "var(--paper-3)"
  },
  accent: {
    color: "var(--red-700)",
    background: "var(--red-100)"
  },
  success: {
    color: "var(--status-open-fg)",
    background: "var(--status-open-bg)"
  },
  warning: {
    color: "var(--status-closed-fg)",
    background: "var(--status-closed-bg)"
  },
  info: {
    color: "var(--status-accepted-fg)",
    background: "var(--status-accepted-bg)"
  },
  outline: {
    color: "var(--text-muted)",
    background: "transparent",
    boxShadow: "inset 0 0 0 1px var(--border-default)"
  }
};

/** Small count / label chip. */
function Badge({
  children,
  tone = "neutral",
  icon,
  uppercase,
  style,
  ...rest
}) {
  return /*#__PURE__*/React.createElement("span", _extends({
    style: {
      display: "inline-flex",
      alignItems: "center",
      gap: 5,
      padding: "2px 8px",
      borderRadius: "var(--radius-pill)",
      font: "var(--weight-semibold) var(--text-micro)/1.5 var(--font-sans)",
      letterSpacing: uppercase ? "var(--tracking-caps)" : "var(--tracking-normal)",
      textTransform: uppercase ? "uppercase" : "none",
      whiteSpace: "nowrap",
      ...tones[tone],
      ...style
    }
  }, rest), icon ? /*#__PURE__*/React.createElement(__ds_scope.Icon, {
    name: icon,
    size: 12
  }) : null, children);
}
Object.assign(__ds_scope, { Badge });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/Badge.jsx", error: String((e && e.message) || e) }); }

// components/core/Button.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const sizes = {
  sm: {
    height: "var(--control-h-sm)",
    padding: "0 10px",
    font: "var(--weight-medium) var(--text-body-sm)/1 var(--font-sans)",
    gap: 6,
    icon: 14
  },
  md: {
    height: "var(--control-h-md)",
    padding: "0 14px",
    font: "var(--weight-medium) var(--text-body-md)/1 var(--font-sans)",
    gap: 8,
    icon: 16
  },
  lg: {
    height: "var(--control-h-lg)",
    padding: "0 20px",
    font: "var(--weight-medium) var(--text-body-lg)/1 var(--font-sans)",
    gap: 8,
    icon: 18
  },
  touch: {
    height: "var(--control-h-touch)",
    padding: "0 24px",
    font: "var(--weight-semibold) var(--text-title-sm)/1 var(--font-sans)",
    gap: 10,
    icon: 20
  }
};
const variants = {
  primary: {
    background: "var(--accent-solid)",
    color: "var(--accent-on-solid)",
    border: "1px solid var(--accent-solid)",
    hover: {
      background: "var(--accent-solid-hover)",
      borderColor: "var(--accent-solid-hover)"
    }
  },
  secondary: {
    background: "var(--surface-card)",
    color: "var(--text-strong)",
    border: "1px solid var(--border-default)",
    hover: {
      background: "var(--surface-hover)",
      borderColor: "var(--border-strong)"
    }
  },
  ghost: {
    background: "transparent",
    color: "var(--text-body)",
    border: "1px solid transparent",
    hover: {
      background: "var(--surface-hover)"
    }
  },
  inverse: {
    background: "var(--surface-inverse)",
    color: "var(--text-inverse)",
    border: "1px solid var(--surface-inverse)",
    hover: {
      background: "var(--ink-700)",
      borderColor: "var(--ink-700)"
    }
  },
  danger: {
    background: "var(--surface-card)",
    color: "var(--red-700)",
    border: "1px solid var(--red-200)",
    hover: {
      background: "var(--red-50)",
      borderColor: "var(--red-400)"
    }
  }
};

/** Primary action control. One primary button per view. */
function Button({
  children,
  variant = "secondary",
  size = "md",
  iconLeft,
  iconRight,
  disabled,
  loading,
  fullWidth,
  type = "button",
  style,
  onClick,
  ...rest
}) {
  const [hover, setHover] = React.useState(false);
  const [press, setPress] = React.useState(false);
  const s = sizes[size] || sizes.md;
  const v = variants[variant] || variants.secondary;
  const off = disabled || loading;
  return /*#__PURE__*/React.createElement("button", _extends({
    type: type,
    disabled: off,
    onClick: off ? undefined : onClick,
    onMouseEnter: () => setHover(true),
    onMouseLeave: () => {
      setHover(false);
      setPress(false);
    },
    onMouseDown: () => setPress(true),
    onMouseUp: () => setPress(false),
    style: {
      display: "inline-flex",
      alignItems: "center",
      justifyContent: "center",
      width: fullWidth ? "100%" : undefined,
      height: s.height,
      padding: s.padding,
      font: s.font,
      gap: s.gap,
      letterSpacing: "var(--tracking-snug)",
      whiteSpace: "nowrap",
      borderRadius: "var(--radius-sm)",
      cursor: off ? "not-allowed" : "pointer",
      opacity: off ? 0.42 : 1,
      transition: "var(--motion-hover), transform var(--dur-instant) var(--ease-out)",
      transform: press && !off ? "translateY(1px)" : "none",
      boxShadow: press && !off ? "var(--shadow-press)" : "none",
      ...v,
      ...(hover && !off ? v.hover : null),
      hover: undefined,
      ...style
    }
  }, rest), loading ? /*#__PURE__*/React.createElement(__ds_scope.Icon, {
    name: "clock",
    size: s.icon
  }) : iconLeft ? /*#__PURE__*/React.createElement(__ds_scope.Icon, {
    name: iconLeft,
    size: s.icon
  }) : null, children, iconRight ? /*#__PURE__*/React.createElement(__ds_scope.Icon, {
    name: iconRight,
    size: s.icon
  }) : null);
}
Object.assign(__ds_scope, { Button });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/Button.jsx", error: String((e && e.message) || e) }); }

// components/core/IconButton.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const box = {
  sm: 30,
  md: 36,
  lg: 44
};

/** Square, label-less action. Always pass an aria-label. */
function IconButton({
  icon,
  size = "md",
  variant = "ghost",
  active,
  disabled,
  label,
  style,
  onClick,
  ...rest
}) {
  const [hover, setHover] = React.useState(false);
  const d = box[size] || box.md;
  const solid = variant === "solid";
  const bg = active ? "var(--surface-active)" : solid ? "var(--accent-solid)" : hover && !disabled ? "var(--surface-hover)" : "transparent";
  return /*#__PURE__*/React.createElement("button", _extends({
    type: "button",
    "aria-label": label,
    title: label,
    disabled: disabled,
    onClick: disabled ? undefined : onClick,
    onMouseEnter: () => setHover(true),
    onMouseLeave: () => setHover(false),
    style: {
      display: "inline-flex",
      alignItems: "center",
      justifyContent: "center",
      width: d,
      height: d,
      borderRadius: "var(--radius-sm)",
      background: bg,
      color: solid ? "var(--accent-on-solid)" : variant === "outline" && hover ? "var(--text-strong)" : "var(--text-muted)",
      border: variant === "outline" ? "1px solid var(--border-default)" : "1px solid transparent",
      cursor: disabled ? "not-allowed" : "pointer",
      opacity: disabled ? 0.4 : 1,
      transition: "var(--motion-hover)",
      ...style
    }
  }, rest), /*#__PURE__*/React.createElement(__ds_scope.Icon, {
    name: icon,
    size: size === "sm" ? 15 : size === "lg" ? 20 : 17
  }));
}
Object.assign(__ds_scope, { IconButton });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/IconButton.jsx", error: String((e && e.message) || e) }); }

// components/core/StatCard.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/** Single KPI tile: label, big numeral, optional delta. */
function StatCard({
  label,
  value,
  unit,
  delta,
  deltaDirection = "up",
  caption,
  icon,
  tone = "default",
  style,
  ...rest
}) {
  const inverse = tone === "inverse";
  const good = deltaDirection === "up";
  return /*#__PURE__*/React.createElement("div", _extends({
    style: {
      display: "grid",
      gap: "var(--space-3)",
      padding: "var(--pad-card)",
      borderRadius: "var(--radius-md)",
      background: inverse ? "var(--surface-inverse)" : "var(--surface-card)",
      border: "1px solid " + (inverse ? "var(--ink-800)" : "var(--border-default)"),
      boxShadow: "var(--shadow-sm)",
      ...style
    }
  }, rest), /*#__PURE__*/React.createElement("div", {
    style: {
      display: "flex",
      alignItems: "center",
      gap: 6,
      color: inverse ? "var(--ink-300)" : "var(--text-muted)"
    }
  }, icon ? /*#__PURE__*/React.createElement(__ds_scope.Icon, {
    name: icon,
    size: 13
  }) : null, /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--type-eyebrow)",
      letterSpacing: "var(--tracking-caps)",
      textTransform: "uppercase"
    }
  }, label)), /*#__PURE__*/React.createElement("div", {
    style: {
      display: "flex",
      alignItems: "baseline",
      gap: 8
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-regular) var(--text-display-md)/1.05 var(--font-display)",
      color: inverse ? "var(--paper-1)" : "var(--text-strong)",
      fontVariantNumeric: "tabular-nums"
    }
  }, value), unit ? /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--type-label)",
      color: inverse ? "var(--ink-300)" : "var(--text-muted)"
    }
  }, unit) : null, delta != null ? /*#__PURE__*/React.createElement("span", {
    style: {
      display: "inline-flex",
      alignItems: "center",
      gap: 3,
      font: "var(--weight-semibold) var(--text-body-sm)/1 var(--font-sans)",
      color: good ? "var(--green-700)" : "var(--red-700)"
    }
  }, /*#__PURE__*/React.createElement(__ds_scope.Icon, {
    name: good ? "trending-up" : "trending-down",
    size: 13
  }), delta) : null), caption ? /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-regular) var(--text-body-sm)/1.4 var(--font-sans)",
      color: inverse ? "var(--ink-300)" : "var(--text-muted)"
    }
  }, caption) : null);
}
Object.assign(__ds_scope, { StatCard });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/StatCard.jsx", error: String((e && e.message) || e) }); }

// components/data/DataTable.jsx
try { (() => {
/** Dense record table. Columns: {key,header,align,width,render}. */
function DataTable({
  columns = [],
  rows = [],
  onRowClick,
  selectedId,
  rowKey = "id",
  dense,
  empty = "Nothing to show yet.",
  style
}) {
  const [hoverRow, setHoverRow] = React.useState(null);
  const padY = dense ? "8px" : "var(--pad-cell-y)";
  return /*#__PURE__*/React.createElement("div", {
    style: {
      width: "100%",
      overflowX: "auto",
      ...style
    }
  }, /*#__PURE__*/React.createElement("table", {
    style: {
      width: "100%",
      borderCollapse: "collapse",
      font: "var(--type-body)"
    }
  }, /*#__PURE__*/React.createElement("thead", null, /*#__PURE__*/React.createElement("tr", null, columns.map(c => /*#__PURE__*/React.createElement("th", {
    key: c.key,
    style: {
      textAlign: c.align || "left",
      width: c.width,
      padding: "9px var(--pad-cell-x)",
      font: "var(--type-eyebrow)",
      letterSpacing: "var(--tracking-caps)",
      textTransform: "uppercase",
      color: "var(--text-muted)",
      background: "var(--paper-1)",
      borderBottom: "1px solid var(--border-default)",
      whiteSpace: "nowrap"
    }
  }, c.header)))), /*#__PURE__*/React.createElement("tbody", null, rows.length === 0 ? /*#__PURE__*/React.createElement("tr", null, /*#__PURE__*/React.createElement("td", {
    colSpan: columns.length,
    style: {
      padding: "var(--space-9)",
      textAlign: "center",
      color: "var(--text-muted)",
      font: "var(--type-body)"
    }
  }, empty)) : rows.map((r, i) => {
    const id = r[rowKey] != null ? r[rowKey] : i;
    const selected = selectedId != null && selectedId === id;
    return /*#__PURE__*/React.createElement("tr", {
      key: id,
      onClick: onRowClick ? () => onRowClick(r) : undefined,
      onMouseEnter: () => setHoverRow(id),
      onMouseLeave: () => setHoverRow(null),
      style: {
        cursor: onRowClick ? "pointer" : "default",
        background: selected ? "var(--red-50)" : hoverRow === id ? "var(--surface-hover)" : "var(--surface-card)",
        boxShadow: selected ? "inset 2px 0 0 var(--accent-solid)" : "none",
        transition: "background-color var(--dur-fast) var(--ease-out)"
      }
    }, columns.map(c => /*#__PURE__*/React.createElement("td", {
      key: c.key,
      style: {
        padding: padY + " var(--pad-cell-x)",
        textAlign: c.align || "left",
        borderBottom: "1px solid var(--border-subtle)",
        color: "var(--text-body)",
        verticalAlign: "middle"
      }
    }, c.render ? c.render(r) : r[c.key])));
  }))));
}
Object.assign(__ds_scope, { DataTable });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/data/DataTable.jsx", error: String((e && e.message) || e) }); }

// components/data/KeyValueList.jsx
try { (() => {
/** Label/value pairs for record detail panels. */
function KeyValueList({
  items = [],
  columns = 1,
  dividers = true,
  style
}) {
  return /*#__PURE__*/React.createElement("dl", {
    style: {
      display: "grid",
      gridTemplateColumns: "repeat(" + columns + ", minmax(0,1fr))",
      gap: "0 var(--space-8)",
      margin: 0,
      ...style
    }
  }, items.map((it, i) => /*#__PURE__*/React.createElement("div", {
    key: it.label + i,
    style: {
      display: "flex",
      alignItems: "baseline",
      justifyContent: "space-between",
      gap: "var(--space-5)",
      padding: "7px 0",
      borderBottom: dividers && i < items.length - (items.length % columns === 0 ? columns : 1) ? "1px dashed var(--border-default)" : "none"
    }
  }, /*#__PURE__*/React.createElement("dt", {
    style: {
      font: "var(--weight-regular) var(--text-body-sm)/1.4 var(--font-sans)",
      color: "var(--text-muted)"
    }
  }, it.label), /*#__PURE__*/React.createElement("dd", {
    style: {
      margin: 0,
      textAlign: "right",
      font: "var(--type-label)",
      color: "var(--text-strong)"
    }
  }, it.value))));
}
Object.assign(__ds_scope, { KeyValueList });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/data/KeyValueList.jsx", error: String((e && e.message) || e) }); }

// components/data/MoneyAmount.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/** Tabular currency figure; signed variances are colour-coded. */
function MoneyAmount({
  value,
  currency = "$",
  size = "md",
  signed,
  tone,
  strong,
  style,
  ...rest
}) {
  const n = typeof value === "number" ? value : parseFloat(value) || 0;
  const auto = signed ? n < 0 ? "negative" : n > 0 ? "positive" : "neutral" : "neutral";
  const key = tone || auto;
  const color = key === "negative" ? "var(--money-negative)" : key === "positive" ? "var(--money-positive)" : key === "muted" ? "var(--text-muted)" : "var(--money-neutral)";
  const fs = size === "lg" ? "var(--text-title-lg)" : size === "sm" ? "var(--text-body-sm)" : "var(--text-body-md)";
  const sign = n < 0 ? "-" : signed && n > 0 ? "+" : "";
  return /*#__PURE__*/React.createElement("span", _extends({
    style: {
      font: (strong ? "var(--weight-semibold) " : "var(--weight-medium) ") + fs + "/1.3 var(--font-mono)",
      fontVariantNumeric: "tabular-nums",
      color,
      whiteSpace: "nowrap",
      ...style
    }
  }, rest), sign, currency, Math.abs(n).toFixed(2));
}
Object.assign(__ds_scope, { MoneyAmount });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/data/MoneyAmount.jsx", error: String((e && e.message) || e) }); }

// components/data/StatusPill.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const map = {
  open: {
    fg: "var(--status-open-fg)",
    bg: "var(--status-open-bg)"
  },
  closed: {
    fg: "var(--status-closed-fg)",
    bg: "var(--status-closed-bg)"
  },
  accepted: {
    fg: "var(--status-accepted-fg)",
    bg: "var(--status-accepted-bg)"
  },
  cancelled: {
    fg: "var(--status-cancelled-fg)",
    bg: "var(--status-cancelled-bg)"
  },
  danger: {
    fg: "var(--status-danger-fg)",
    bg: "var(--status-danger-bg)"
  }
};

/** Lifecycle state of a shift, order or ticket. */
function StatusPill({
  status,
  label,
  dot = true,
  style,
  ...rest
}) {
  const c = map[status] || map.cancelled;
  return /*#__PURE__*/React.createElement("span", _extends({
    style: {
      display: "inline-flex",
      alignItems: "center",
      gap: 6,
      padding: "3px 9px",
      borderRadius: "var(--radius-pill)",
      background: c.bg,
      color: c.fg,
      font: "var(--weight-semibold) var(--text-micro)/1.4 var(--font-sans)",
      letterSpacing: "var(--tracking-caps)",
      textTransform: "uppercase",
      whiteSpace: "nowrap",
      ...style
    }
  }, rest), dot ? /*#__PURE__*/React.createElement("span", {
    style: {
      width: 5,
      height: 5,
      borderRadius: "50%",
      background: "currentColor"
    }
  }) : null, label || status);
}
Object.assign(__ds_scope, { StatusPill });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/data/StatusPill.jsx", error: String((e && e.message) || e) }); }

// components/feedback/AIInsight.jsx
try { (() => {
/** AI recommendation surface: always shows confidence, reasoning and its source. */
function AIInsight({
  title,
  body,
  confidence,
  basis,
  actions,
  onAccept,
  onDismiss,
  acceptLabel = "Apply",
  requiresConfirmation,
  compact,
  style
}) {
  const pct = confidence != null ? Math.round(confidence * 100) : null;
  const level = pct == null ? null : pct >= 80 ? "High" : pct >= 55 ? "Medium" : "Low";
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gap: compact ? "var(--space-4)" : "var(--space-5)",
      padding: compact ? "var(--pad-card-tight)" : "var(--pad-card)",
      background: "var(--ai-surface)",
      border: "1px solid var(--ai-border)",
      borderRadius: "var(--radius-md)",
      ...style
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: "flex",
      alignItems: "center",
      gap: 8
    }
  }, /*#__PURE__*/React.createElement(__ds_scope.Icon, {
    name: "sparkles",
    size: 14,
    style: {
      color: "var(--ai-marker)"
    }
  }), /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--type-eyebrow)",
      letterSpacing: "var(--tracking-caps)",
      textTransform: "uppercase",
      color: "var(--text-muted)"
    }
  }, "AIVO suggests"), pct != null ? /*#__PURE__*/React.createElement("span", {
    style: {
      marginLeft: "auto",
      display: "inline-flex",
      alignItems: "center",
      gap: 7
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      width: 46,
      height: 4,
      borderRadius: 2,
      background: "var(--paper-4)",
      overflow: "hidden"
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      display: "block",
      width: pct + "%",
      height: "100%",
      background: "var(--ink-800)"
    }
  })), /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-medium) var(--text-micro)/1 var(--font-mono)",
      color: "var(--text-muted)"
    }
  }, level, " \xB7 ", pct, "%")) : null), /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gap: 4
    }
  }, title ? /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-medium) var(--text-title-sm)/1.35 var(--font-sans)",
      color: "var(--text-strong)"
    }
  }, title) : null, body ? /*#__PURE__*/React.createElement("p", {
    style: {
      font: "var(--weight-regular) var(--text-body-md)/var(--leading-relaxed) var(--font-sans)",
      color: "var(--text-body)",
      maxWidth: "62ch"
    }
  }, body) : null), basis ? /*#__PURE__*/React.createElement("p", {
    style: {
      font: "var(--weight-regular) var(--text-caption)/1.5 var(--font-sans)",
      color: "var(--text-muted)"
    }
  }, "Based on ", basis) : null, actions || onAccept || onDismiss ? /*#__PURE__*/React.createElement("div", {
    style: {
      display: "flex",
      alignItems: "center",
      gap: "var(--gap-inline)",
      flexWrap: "wrap"
    }
  }, onAccept ? /*#__PURE__*/React.createElement(__ds_scope.Button, {
    size: "sm",
    variant: "inverse",
    onClick: onAccept
  }, requiresConfirmation ? "Review & " + acceptLabel.toLowerCase() : acceptLabel) : null, onDismiss ? /*#__PURE__*/React.createElement(__ds_scope.Button, {
    size: "sm",
    variant: "ghost",
    onClick: onDismiss
  }, "Dismiss") : null, actions, requiresConfirmation ? /*#__PURE__*/React.createElement("span", {
    style: {
      display: "inline-flex",
      alignItems: "center",
      gap: 5,
      font: "var(--weight-regular) var(--text-caption)/1 var(--font-sans)",
      color: "var(--text-muted)"
    }
  }, /*#__PURE__*/React.createElement(__ds_scope.Icon, {
    name: "lock",
    size: 12
  }), " needs your confirmation") : null) : null);
}
Object.assign(__ds_scope, { AIInsight });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/feedback/AIInsight.jsx", error: String((e && e.message) || e) }); }

// components/feedback/Dialog.jsx
try { (() => {
/** Centred modal for confirmations and focused review. */
function Dialog({
  open = true,
  title,
  description,
  children,
  footer,
  onClose,
  width = 480,
  tone = "default",
  style
}) {
  if (!open) return null;
  return /*#__PURE__*/React.createElement("div", {
    style: {
      position: "fixed",
      inset: 0,
      zIndex: 60,
      display: "grid",
      placeItems: "center",
      padding: "var(--space-8)",
      background: "var(--scrim)",
      backdropFilter: "var(--blur-overlay)"
    },
    onClick: onClose
  }, /*#__PURE__*/React.createElement("div", {
    role: "dialog",
    "aria-modal": "true",
    onClick: e => e.stopPropagation(),
    style: {
      width: "100%",
      maxWidth: width,
      background: "var(--surface-card)",
      border: "1px solid var(--border-default)",
      borderRadius: "var(--radius-lg)",
      boxShadow: "var(--shadow-overlay)",
      overflow: "hidden",
      ...style
    }
  }, /*#__PURE__*/React.createElement("header", {
    style: {
      display: "flex",
      alignItems: "flex-start",
      gap: "var(--space-5)",
      padding: "var(--space-7) var(--space-7) var(--space-5)"
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gap: 6,
      flex: 1
    }
  }, title ? /*#__PURE__*/React.createElement("h2", {
    style: {
      font: "var(--weight-regular) var(--text-title-lg)/1.25 var(--font-display)",
      color: tone === "danger" ? "var(--red-800)" : "var(--text-strong)"
    }
  }, title) : null, description ? /*#__PURE__*/React.createElement("p", {
    style: {
      font: "var(--weight-regular) var(--text-body-md)/var(--leading-relaxed) var(--font-sans)",
      color: "var(--text-muted)"
    }
  }, description) : null), onClose ? /*#__PURE__*/React.createElement(__ds_scope.IconButton, {
    icon: "x",
    label: "Close",
    onClick: onClose
  }) : null), children ? /*#__PURE__*/React.createElement("div", {
    style: {
      padding: "0 var(--space-7) var(--space-7)"
    }
  }, children) : null, footer ? /*#__PURE__*/React.createElement("footer", {
    style: {
      display: "flex",
      justifyContent: "flex-end",
      gap: "var(--gap-inline)",
      padding: "var(--space-5) var(--space-7)",
      borderTop: "1px solid var(--border-subtle)",
      background: "var(--paper-1)"
    }
  }, footer) : null));
}
Object.assign(__ds_scope, { Dialog });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/feedback/Dialog.jsx", error: String((e && e.message) || e) }); }

// components/feedback/EmptyState.jsx
try { (() => {
/** Zero-state for empty lists and unconfigured areas. */
function EmptyState({
  icon = "coffee",
  title,
  message,
  action,
  compact,
  style
}) {
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      justifyItems: "center",
      gap: "var(--space-5)",
      textAlign: "center",
      padding: compact ? "var(--space-8)" : "var(--space-11) var(--space-8)",
      ...style
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      display: "grid",
      placeItems: "center",
      width: 40,
      height: 40,
      borderRadius: "var(--radius-pill)",
      background: "var(--paper-2)",
      color: "var(--text-subtle)"
    }
  }, /*#__PURE__*/React.createElement(__ds_scope.Icon, {
    name: icon,
    size: 19
  })), /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gap: 5
    }
  }, title ? /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-regular) var(--text-title-md)/1.3 var(--font-display)",
      color: "var(--text-strong)"
    }
  }, title) : null, message ? /*#__PURE__*/React.createElement("p", {
    style: {
      font: "var(--weight-regular) var(--text-body-md)/var(--leading-relaxed) var(--font-sans)",
      color: "var(--text-muted)",
      maxWidth: "46ch"
    }
  }, message) : null), action);
}
Object.assign(__ds_scope, { EmptyState });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/feedback/EmptyState.jsx", error: String((e && e.message) || e) }); }

// components/feedback/Toast.jsx
try { (() => {
const tones = {
  info: {
    icon: "info",
    fg: "var(--ink-800)",
    accent: "var(--ink-800)"
  },
  success: {
    icon: "circle-check",
    fg: "var(--ink-800)",
    accent: "var(--green-500)"
  },
  warning: {
    icon: "triangle-alert",
    fg: "var(--ink-800)",
    accent: "var(--amber-500)"
  },
  danger: {
    icon: "circle-alert",
    fg: "var(--ink-800)",
    accent: "var(--red-600)"
  }
};

/** Transient confirmation of a completed action. */
function Toast({
  tone = "info",
  title,
  message,
  action,
  onDismiss,
  style
}) {
  const t = tones[tone] || tones.info;
  return /*#__PURE__*/React.createElement("div", {
    role: "status",
    style: {
      display: "flex",
      alignItems: "flex-start",
      gap: "var(--space-5)",
      width: 380,
      padding: "var(--space-5) var(--space-6)",
      background: "var(--surface-card)",
      border: "1px solid var(--border-default)",
      borderRadius: "var(--radius-md)",
      boxShadow: "var(--shadow-lg)",
      ...style
    }
  }, /*#__PURE__*/React.createElement(__ds_scope.Icon, {
    name: t.icon,
    size: 17,
    style: {
      color: t.accent,
      marginTop: 1
    }
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gap: 3,
      flex: 1
    }
  }, title ? /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-semibold) var(--text-body-md)/1.35 var(--font-sans)",
      color: t.fg
    }
  }, title) : null, message ? /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-regular) var(--text-body-sm)/1.5 var(--font-sans)",
      color: "var(--text-muted)"
    }
  }, message) : null, action ? /*#__PURE__*/React.createElement("div", {
    style: {
      marginTop: 4
    }
  }, action) : null), onDismiss ? /*#__PURE__*/React.createElement(__ds_scope.IconButton, {
    icon: "x",
    label: "Dismiss",
    size: "sm",
    onClick: onDismiss
  }) : null);
}
Object.assign(__ds_scope, { Toast });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/feedback/Toast.jsx", error: String((e && e.message) || e) }); }

// components/feedback/Tooltip.jsx
try { (() => {
/** Hover/focus explanation for terse controls and domain terms. */
function Tooltip({
  label,
  placement = "top",
  children,
  style
}) {
  const [show, setShow] = React.useState(false);
  const pos = placement === "bottom" ? {
    top: "calc(100% + 6px)",
    left: "50%",
    transform: "translateX(-50%)"
  } : placement === "right" ? {
    left: "calc(100% + 6px)",
    top: "50%",
    transform: "translateY(-50%)"
  } : {
    bottom: "calc(100% + 6px)",
    left: "50%",
    transform: "translateX(-50%)"
  };
  return /*#__PURE__*/React.createElement("span", {
    style: {
      position: "relative",
      display: "inline-flex",
      ...style
    },
    onMouseEnter: () => setShow(true),
    onMouseLeave: () => setShow(false),
    onFocus: () => setShow(true),
    onBlur: () => setShow(false)
  }, children, /*#__PURE__*/React.createElement("span", {
    role: "tooltip",
    style: {
      position: "absolute",
      zIndex: 40,
      ...pos,
      padding: "5px 9px",
      borderRadius: "var(--radius-sm)",
      background: "var(--surface-inverse)",
      color: "var(--text-inverse)",
      font: "var(--weight-regular) var(--text-caption)/1.4 var(--font-sans)",
      whiteSpace: "nowrap",
      pointerEvents: "none",
      opacity: show ? 1 : 0,
      transition: "opacity var(--dur-fast) var(--ease-out)",
      boxShadow: "var(--shadow-md)"
    }
  }, label));
}
Object.assign(__ds_scope, { Tooltip });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/feedback/Tooltip.jsx", error: String((e && e.message) || e) }); }

// components/forms/Checkbox.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/** Checkbox with inline label and optional description. */
function Checkbox({
  label,
  description,
  checked,
  defaultChecked,
  onChange,
  disabled,
  style,
  ...rest
}) {
  const [inner, setInner] = React.useState(!!defaultChecked);
  const on = checked != null ? checked : inner;
  return /*#__PURE__*/React.createElement("label", {
    style: {
      display: "flex",
      alignItems: description ? "flex-start" : "center",
      gap: 10,
      cursor: disabled ? "not-allowed" : "pointer",
      opacity: disabled ? 0.5 : 1,
      ...style
    }
  }, /*#__PURE__*/React.createElement("input", _extends({
    type: "checkbox",
    checked: on,
    disabled: disabled,
    onChange: e => {
      if (checked == null) setInner(e.target.checked);
      if (onChange) onChange(e);
    },
    style: {
      position: "absolute",
      opacity: 0,
      width: 0,
      height: 0
    }
  }, rest)), /*#__PURE__*/React.createElement("span", {
    style: {
      display: "inline-flex",
      alignItems: "center",
      justifyContent: "center",
      flex: "none",
      width: 18,
      height: 18,
      marginTop: description ? 1 : 0,
      borderRadius: "var(--radius-xs)",
      background: on ? "var(--accent-solid)" : "var(--surface-card)",
      border: "1px solid " + (on ? "var(--accent-solid)" : "var(--border-strong)"),
      color: "var(--accent-on-solid)",
      transition: "var(--motion-hover)"
    }
  }, on ? /*#__PURE__*/React.createElement(__ds_scope.Icon, {
    name: "check",
    size: 13
  }) : null), /*#__PURE__*/React.createElement("span", {
    style: {
      display: "grid",
      gap: 2
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--type-body)",
      color: "var(--text-body)"
    }
  }, label), description ? /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-regular) var(--text-caption)/1.45 var(--font-sans)",
      color: "var(--text-muted)"
    }
  }, description) : null));
}
Object.assign(__ds_scope, { Checkbox });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Checkbox.jsx", error: String((e && e.message) || e) }); }

// components/forms/Input.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const heights = {
  sm: "var(--control-h-sm)",
  md: "var(--control-h-md)",
  lg: "var(--control-h-lg)"
};

/** Text / number field with label, prefix and hint. */
function Input({
  label,
  hint,
  error,
  prefix,
  suffix,
  icon,
  size = "md",
  align,
  required,
  id,
  style,
  wrapperStyle,
  ...rest
}) {
  const [focus, setFocus] = React.useState(false);
  const fid = id || React.useId();
  const border = error ? "var(--red-500)" : focus ? "var(--border-focus)" : "var(--border-default)";
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gap: 6,
      ...wrapperStyle
    }
  }, label ? /*#__PURE__*/React.createElement("label", {
    htmlFor: fid,
    style: {
      font: "var(--type-label)",
      color: "var(--text-body)"
    }
  }, label, required ? /*#__PURE__*/React.createElement("span", {
    style: {
      color: "var(--red-600)"
    }
  }, " *") : null) : null, /*#__PURE__*/React.createElement("div", {
    style: {
      display: "flex",
      alignItems: "center",
      gap: 8,
      height: heights[size],
      padding: "0 12px",
      background: "var(--surface-card)",
      border: "1px solid " + border,
      borderRadius: "var(--radius-sm)",
      boxShadow: focus ? "var(--focus-ring)" : "none",
      transition: "var(--motion-hover), box-shadow var(--dur-fast) var(--ease-out)"
    }
  }, icon ? /*#__PURE__*/React.createElement(__ds_scope.Icon, {
    name: icon,
    size: 15,
    style: {
      color: "var(--text-subtle)"
    }
  }) : null, prefix ? /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--type-label)",
      color: "var(--text-muted)"
    }
  }, prefix) : null, /*#__PURE__*/React.createElement("input", _extends({
    id: fid,
    onFocus: () => setFocus(true),
    onBlur: () => setFocus(false),
    style: {
      flex: 1,
      minWidth: 0,
      border: "none",
      outline: "none",
      background: "transparent",
      font: rest.type === "number" ? "var(--type-numeric)" : "var(--type-body)",
      color: "var(--text-strong)",
      textAlign: align || (rest.type === "number" ? "right" : "left"),
      ...style
    }
  }, rest)), suffix ? /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--type-label)",
      color: "var(--text-muted)"
    }
  }, suffix) : null), error || hint ? /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-regular) var(--text-caption)/1.4 var(--font-sans)",
      color: error ? "var(--red-700)" : "var(--text-muted)"
    }
  }, error || hint) : null);
}
Object.assign(__ds_scope, { Input });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Input.jsx", error: String((e && e.message) || e) }); }

// components/forms/QuantityStepper.jsx
try { (() => {
/** Touch-friendly quantity control for POS / waiter surfaces. */
function QuantityStepper({
  value,
  defaultValue = 1,
  min = 0,
  max = 99,
  onChange,
  size = "md",
  style
}) {
  const [inner, setInner] = React.useState(defaultValue);
  const v = value != null ? value : inner;
  const set = n => {
    const c = Math.min(max, Math.max(min, n));
    if (value == null) setInner(c);
    if (onChange) onChange(c);
  };
  const d = size === "lg" ? 44 : 32;
  const btn = (icon, delta, disabled) => /*#__PURE__*/React.createElement("button", {
    type: "button",
    "aria-label": delta > 0 ? "Increase" : "Decrease",
    disabled: disabled,
    onClick: () => set(v + delta),
    style: {
      width: d,
      height: d,
      display: "grid",
      placeItems: "center",
      border: "none",
      background: "transparent",
      color: disabled ? "var(--text-subtle)" : "var(--text-strong)",
      cursor: disabled ? "not-allowed" : "pointer",
      borderRadius: "var(--radius-sm)"
    }
  }, /*#__PURE__*/React.createElement(__ds_scope.Icon, {
    name: icon,
    size: size === "lg" ? 20 : 16
  }));
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: "inline-flex",
      alignItems: "center",
      background: "var(--surface-card)",
      border: "1px solid var(--border-default)",
      borderRadius: "var(--radius-sm)",
      overflow: "hidden",
      ...style
    }
  }, btn("minus", -1, v <= min), /*#__PURE__*/React.createElement("span", {
    style: {
      minWidth: d,
      textAlign: "center",
      font: "var(--weight-medium) " + (size === "lg" ? "var(--text-title-md)" : "var(--text-body-md)") + "/1 var(--font-mono)",
      color: "var(--text-strong)",
      fontVariantNumeric: "tabular-nums"
    }
  }, v), btn("plus", 1, v >= max));
}
Object.assign(__ds_scope, { QuantityStepper });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/QuantityStepper.jsx", error: String((e && e.message) || e) }); }

// components/forms/Select.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const heights = {
  sm: "var(--control-h-sm)",
  md: "var(--control-h-md)",
  lg: "var(--control-h-lg)"
};

/** Native select styled to match Input. */
function Select({
  label,
  hint,
  options = [],
  size = "md",
  disabled,
  id,
  wrapperStyle,
  style,
  ...rest
}) {
  const fid = id || React.useId();
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gap: 6,
      ...wrapperStyle
    }
  }, label ? /*#__PURE__*/React.createElement("label", {
    htmlFor: fid,
    style: {
      font: "var(--type-label)",
      color: "var(--text-body)"
    }
  }, label) : null, /*#__PURE__*/React.createElement("div", {
    style: {
      position: "relative",
      display: "flex",
      alignItems: "center",
      opacity: disabled ? 0.5 : 1
    }
  }, /*#__PURE__*/React.createElement("select", _extends({
    id: fid,
    disabled: disabled,
    style: {
      appearance: "none",
      WebkitAppearance: "none",
      width: "100%",
      height: heights[size],
      padding: "0 32px 0 12px",
      background: "var(--surface-card)",
      border: "1px solid var(--border-default)",
      borderRadius: "var(--radius-sm)",
      font: "var(--type-body)",
      color: "var(--text-strong)",
      cursor: disabled ? "not-allowed" : "pointer",
      ...style
    }
  }, rest), options.map(o => {
    const opt = typeof o === "string" ? {
      value: o,
      label: o
    } : o;
    return /*#__PURE__*/React.createElement("option", {
      key: opt.value,
      value: opt.value
    }, opt.label);
  })), /*#__PURE__*/React.createElement(__ds_scope.Icon, {
    name: "chevron-down",
    size: 15,
    style: {
      position: "absolute",
      right: 10,
      color: "var(--text-subtle)",
      pointerEvents: "none"
    }
  })), hint ? /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-regular) var(--text-caption)/1.4 var(--font-sans)",
      color: "var(--text-muted)"
    }
  }, hint) : null);
}
Object.assign(__ds_scope, { Select });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Select.jsx", error: String((e && e.message) || e) }); }

// components/forms/Switch.jsx
try { (() => {
/** Immediate on/off setting. */
function Switch({
  label,
  description,
  checked,
  defaultChecked,
  onChange,
  disabled,
  style
}) {
  const [inner, setInner] = React.useState(!!defaultChecked);
  const on = checked != null ? checked : inner;
  const toggle = () => {
    if (disabled) return;
    const next = !on;
    if (checked == null) setInner(next);
    if (onChange) onChange(next);
  };
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: "flex",
      alignItems: "center",
      justifyContent: "space-between",
      gap: "var(--space-6)",
      opacity: disabled ? 0.5 : 1,
      ...style
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      display: "grid",
      gap: 2
    }
  }, label ? /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--type-label)",
      color: "var(--text-strong)"
    }
  }, label) : null, description ? /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-regular) var(--text-caption)/1.45 var(--font-sans)",
      color: "var(--text-muted)"
    }
  }, description) : null), /*#__PURE__*/React.createElement("button", {
    type: "button",
    role: "switch",
    "aria-checked": on,
    "aria-label": typeof label === "string" ? label : "Toggle",
    onClick: toggle,
    disabled: disabled,
    style: {
      flex: "none",
      position: "relative",
      width: 40,
      height: 23,
      padding: 0,
      borderRadius: "var(--radius-pill)",
      cursor: disabled ? "not-allowed" : "pointer",
      background: on ? "var(--accent-solid)" : "var(--paper-4)",
      border: "1px solid " + (on ? "var(--accent-solid)" : "var(--border-strong)"),
      transition: "var(--motion-hover)"
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      position: "absolute",
      top: 2,
      left: on ? 19 : 2,
      width: 17,
      height: 17,
      borderRadius: "var(--radius-pill)",
      background: "#fff",
      boxShadow: "var(--shadow-sm)",
      transition: "left var(--dur-fast) var(--ease-out)"
    }
  })));
}
Object.assign(__ds_scope, { Switch });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Switch.jsx", error: String((e && e.message) || e) }); }

// components/navigation/SidebarNav.jsx
try { (() => {
/** Backoffice left rail: wordmark, nav groups, footer slot. */
function SidebarNav({
  items = [],
  activeId,
  onSelect,
  footer,
  venue,
  collapsed,
  style
}) {
  return /*#__PURE__*/React.createElement("nav", {
    style: {
      display: "flex",
      flexDirection: "column",
      width: collapsed ? "var(--sidebar-w-collapsed)" : "var(--sidebar-w)",
      flex: "none",
      height: "100%",
      background: "var(--paper-1)",
      borderRight: "1px solid var(--border-default)",
      ...style
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: "flex",
      alignItems: "center",
      gap: 10,
      height: "var(--topbar-h)",
      padding: "0 var(--space-6)",
      borderBottom: "1px solid var(--border-subtle)"
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-semibold) var(--text-title-md)/1 var(--font-sans)",
      letterSpacing: "-0.03em",
      color: "var(--text-strong)"
    }
  }, "aivo", /*#__PURE__*/React.createElement("span", {
    style: {
      color: "var(--accent-solid)"
    }
  }, ".")), !collapsed && venue ? /*#__PURE__*/React.createElement("span", {
    style: {
      marginLeft: "auto",
      font: "var(--weight-regular) var(--text-caption)/1 var(--font-sans)",
      color: "var(--text-muted)"
    }
  }, venue) : null), /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1,
      overflowY: "auto",
      padding: "var(--space-5) var(--space-4)"
    }
  }, items.map((item, i) => item.group ? /*#__PURE__*/React.createElement("div", {
    key: "g" + i,
    style: {
      padding: "var(--space-6) var(--space-4) var(--space-3)",
      font: "var(--type-eyebrow)",
      letterSpacing: "var(--tracking-caps)",
      textTransform: "uppercase",
      color: "var(--text-subtle)"
    }
  }, collapsed ? "" : item.group) : /*#__PURE__*/React.createElement(NavItem, {
    key: item.id,
    item: item,
    active: item.id === activeId,
    collapsed: collapsed,
    onSelect: onSelect
  }))), footer ? /*#__PURE__*/React.createElement("div", {
    style: {
      padding: "var(--space-5)",
      borderTop: "1px solid var(--border-subtle)"
    }
  }, footer) : null);
}
function NavItem({
  item,
  active,
  collapsed,
  onSelect
}) {
  const [hover, setHover] = React.useState(false);
  return /*#__PURE__*/React.createElement("button", {
    type: "button",
    onClick: () => onSelect && onSelect(item.id),
    onMouseEnter: () => setHover(true),
    onMouseLeave: () => setHover(false),
    title: collapsed ? item.label : undefined,
    style: {
      display: "flex",
      alignItems: "center",
      gap: 10,
      width: "100%",
      height: 34,
      padding: collapsed ? "0" : "0 10px",
      justifyContent: collapsed ? "center" : "flex-start",
      marginBottom: 2,
      border: "none",
      borderRadius: "var(--radius-sm)",
      background: active ? "var(--surface-card)" : hover ? "var(--surface-hover)" : "transparent",
      boxShadow: active ? "var(--shadow-hairline), inset 0 0 0 1px var(--border-subtle)" : "none",
      color: active ? "var(--text-strong)" : "var(--text-muted)",
      font: (active ? "var(--weight-medium) " : "var(--weight-regular) ") + "var(--text-body-md)/1 var(--font-sans)",
      cursor: "pointer",
      textAlign: "left",
      transition: "var(--motion-hover)"
    }
  }, /*#__PURE__*/React.createElement(__ds_scope.Icon, {
    name: item.icon,
    size: 16,
    style: {
      color: active ? "var(--accent-solid)" : "inherit"
    }
  }), !collapsed ? /*#__PURE__*/React.createElement("span", {
    style: {
      flex: 1
    }
  }, item.label) : null, !collapsed && item.badge != null ? /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-semibold) var(--text-micro)/1 var(--font-mono)",
      color: "var(--red-700)",
      background: "var(--red-100)",
      padding: "3px 6px",
      borderRadius: "var(--radius-pill)"
    }
  }, item.badge) : null);
}
Object.assign(__ds_scope, { SidebarNav });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/navigation/SidebarNav.jsx", error: String((e && e.message) || e) }); }

// components/navigation/Tabs.jsx
try { (() => {
/** Section switcher; underline-marked, ink on paper. */
function Tabs({
  tabs = [],
  activeId,
  onSelect,
  size = "md",
  style
}) {
  return /*#__PURE__*/React.createElement("div", {
    role: "tablist",
    style: {
      display: "flex",
      alignItems: "center",
      gap: "var(--space-7)",
      borderBottom: "1px solid var(--border-default)",
      ...style
    }
  }, tabs.map(t => {
    const active = t.id === activeId;
    return /*#__PURE__*/React.createElement("button", {
      key: t.id,
      role: "tab",
      "aria-selected": active,
      type: "button",
      onClick: () => onSelect && onSelect(t.id),
      style: {
        display: "inline-flex",
        alignItems: "center",
        gap: 7,
        padding: size === "sm" ? "8px 0" : "11px 0",
        border: "none",
        background: "transparent",
        borderBottom: "2px solid " + (active ? "var(--accent-solid)" : "transparent"),
        marginBottom: -1,
        color: active ? "var(--text-strong)" : "var(--text-muted)",
        font: (active ? "var(--weight-medium) " : "var(--weight-regular) ") + "var(--text-body-md)/1 var(--font-sans)",
        cursor: "pointer",
        transition: "var(--motion-hover)"
      }
    }, t.label, t.count != null ? /*#__PURE__*/React.createElement("span", {
      style: {
        font: "var(--weight-medium) var(--text-micro)/1 var(--font-mono)",
        color: "var(--text-muted)",
        background: "var(--paper-2)",
        padding: "3px 6px",
        borderRadius: "var(--radius-pill)"
      }
    }, t.count) : null);
  }));
}
Object.assign(__ds_scope, { Tabs });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/navigation/Tabs.jsx", error: String((e && e.message) || e) }); }

// components/navigation/TopBar.jsx
try { (() => {
/** Page header rail: title, breadcrumb, right-side actions. */
function TopBar({
  title,
  subtitle,
  breadcrumb,
  actions,
  sticky,
  style
}) {
  return /*#__PURE__*/React.createElement("header", {
    style: {
      display: "flex",
      alignItems: "center",
      gap: "var(--space-6)",
      minHeight: "var(--topbar-h)",
      padding: "var(--space-5) var(--gutter-page)",
      background: "var(--paper-1)",
      borderBottom: "1px solid var(--border-default)",
      position: sticky ? "sticky" : "static",
      top: 0,
      zIndex: 20,
      ...style
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gap: 3,
      minWidth: 0
    }
  }, breadcrumb ? /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-regular) var(--text-caption)/1 var(--font-sans)",
      color: "var(--text-subtle)"
    }
  }, breadcrumb) : null, title ? /*#__PURE__*/React.createElement("h1", {
    style: {
      font: "var(--weight-regular) var(--text-title-lg)/1.2 var(--font-display)",
      color: "var(--text-strong)",
      letterSpacing: "var(--tracking-snug)"
    }
  }, title) : null, subtitle ? /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-regular) var(--text-body-sm)/1.4 var(--font-sans)",
      color: "var(--text-muted)"
    }
  }, subtitle) : null), actions ? /*#__PURE__*/React.createElement("div", {
    style: {
      marginLeft: "auto",
      display: "flex",
      alignItems: "center",
      gap: "var(--gap-inline)"
    }
  }, actions) : null);
}
Object.assign(__ds_scope, { TopBar });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/navigation/TopBar.jsx", error: String((e && e.message) || e) }); }

// ui_kits/backoffice/MenuScreen.jsx
try { (() => {
const {
  Card,
  Button,
  IconButton,
  Badge,
  DataTable,
  MoneyAmount,
  TopBar,
  AIInsight,
  Input,
  Switch,
  Tabs
} = window.AIVODesignSystem_3d538f;
function MenuScreen({
  data
}) {
  const [q, setQ] = React.useState("");
  const [cat, setCat] = React.useState("all");
  const cats = ["all"].concat(Array.from(new Set(data.menu.map(m => m.category))));
  const rows = data.menu.filter(m => (cat === "all" || m.category === cat) && m.name.toLowerCase().includes(q.toLowerCase()));
  return /*#__PURE__*/React.createElement("div", null, /*#__PURE__*/React.createElement(TopBar, {
    breadcrumb: "Back office / Menu",
    title: "Menu",
    subtitle: data.menu.length + " items · 4 categories · prices include VAT",
    actions: /*#__PURE__*/React.createElement(React.Fragment, null, /*#__PURE__*/React.createElement(IconButton, {
      icon: "printer",
      label: "Print menu"
    }), /*#__PURE__*/React.createElement(Button, {
      variant: "secondary",
      iconLeft: "pencil"
    }, "Edit categories"), /*#__PURE__*/React.createElement(Button, {
      variant: "primary",
      iconLeft: "plus"
    }, "New item"))
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      padding: "var(--space-7) var(--gutter-page)",
      display: "grid",
      gap: "var(--space-6)",
      maxWidth: "var(--content-max)"
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: "flex",
      alignItems: "flex-end",
      gap: "var(--space-6)"
    }
  }, /*#__PURE__*/React.createElement(Input, {
    wrapperStyle: {
      width: 280
    },
    icon: "search",
    placeholder: "Search items",
    value: q,
    onChange: e => setQ(e.target.value)
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1
    }
  }, /*#__PURE__*/React.createElement(Tabs, {
    size: "sm",
    activeId: cat,
    onSelect: setCat,
    tabs: cats.map(c => ({
      id: c,
      label: c === "all" ? "All" : c
    }))
  }))), /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gridTemplateColumns: "1.6fr 1fr",
      gap: "var(--space-6)",
      alignItems: "start"
    }
  }, /*#__PURE__*/React.createElement(Card, {
    padding: 0
  }, /*#__PURE__*/React.createElement(DataTable, {
    rows: rows,
    empty: "No items match.",
    columns: [{
      key: "name",
      header: "Item",
      render: r => /*#__PURE__*/React.createElement("span", {
        style: {
          display: "grid",
          gap: 3
        }
      }, /*#__PURE__*/React.createElement("span", {
        style: {
          font: "var(--type-label)",
          color: "var(--text-strong)"
        }
      }, r.name), r.flag ? /*#__PURE__*/React.createElement("span", {
        style: {
          font: "var(--weight-regular) var(--text-caption)/1.3 var(--font-sans)",
          color: "var(--text-muted)"
        }
      }, r.flag) : null)
    }, {
      key: "category",
      header: "Category",
      render: r => /*#__PURE__*/React.createElement(Badge, {
        tone: "outline"
      }, r.category)
    }, {
      key: "price",
      header: "Price",
      align: "right",
      render: r => /*#__PURE__*/React.createElement(MoneyAmount, {
        value: r.price
      })
    }, {
      key: "cost",
      header: "Food cost",
      align: "right",
      render: r => /*#__PURE__*/React.createElement(MoneyAmount, {
        value: r.cost,
        tone: "muted"
      })
    }, {
      key: "margin",
      header: "Margin",
      align: "right",
      render: r => /*#__PURE__*/React.createElement("span", {
        style: {
          font: "var(--type-numeric)",
          color: (r.price - r.cost) / r.price > 0.7 ? "var(--money-positive)" : "var(--amber-700)"
        }
      }, Math.round((r.price - r.cost) / r.price * 100), "%")
    }, {
      key: "sold",
      header: "Sold 7d",
      align: "right",
      render: r => /*#__PURE__*/React.createElement("span", {
        style: {
          font: "var(--type-numeric)"
        }
      }, r.sold)
    }]
  })), /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gap: "var(--space-6)"
    }
  }, /*#__PURE__*/React.createElement(AIInsight, {
    title: "Raise burrata & peach to $16",
    body: "Its 58% margin is the lowest on the menu, and covers ordering it have grown 9% week over week.",
    confidence: 0.64,
    basis: "4 weeks of item sales and supplier cost changes",
    requiresConfirmation: true,
    acceptLabel: "Update price",
    onAccept: () => {},
    onDismiss: () => {}
  }), /*#__PURE__*/React.createElement(Card, {
    title: "Availability",
    eyebrow: "Tonight"
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gap: "var(--space-5)"
    }
  }, /*#__PURE__*/React.createElement(Switch, {
    label: "Lamb shoulder",
    description: "18 portions left",
    defaultChecked: true
  }), /*#__PURE__*/React.createElement(Switch, {
    label: "Burrata & peach",
    description: "9 portions left",
    defaultChecked: true
  }), /*#__PURE__*/React.createElement(Switch, {
    label: "Tiramisu",
    description: "Off \u2014 no mascarpone delivery"
  })))))));
}
Object.assign(window, {
  MenuScreen
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/backoffice/MenuScreen.jsx", error: String((e && e.message) || e) }); }

// ui_kits/backoffice/ShiftReviewScreen.jsx
try { (() => {
const {
  Card,
  Button,
  IconButton,
  Badge,
  Icon,
  DataTable,
  StatusPill,
  MoneyAmount,
  KeyValueList,
  Tabs,
  TopBar,
  Select,
  Dialog,
  AIInsight,
  EmptyState,
  Tooltip
} = window.AIVODesignSystem_3d538f;
function GLLineRow({
  line,
  onAccount
}) {
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: "flex",
      alignItems: "center",
      gap: "var(--space-5)",
      padding: "10px 0",
      borderBottom: "1px dashed var(--border-default)"
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      flex: 1,
      font: "var(--type-body)",
      color: "var(--text-body)",
      display: "inline-flex",
      alignItems: "center",
      gap: 6
    }
  }, line.label, line.locked ? /*#__PURE__*/React.createElement(Tooltip, {
    label: "Variance cannot be removed, only redirected"
  }, /*#__PURE__*/React.createElement(Icon, {
    name: "lock",
    size: 12,
    style: {
      color: "var(--red-600)"
    }
  })) : null), /*#__PURE__*/React.createElement(Select, {
    wrapperStyle: {
      width: 190
    },
    size: "sm",
    value: line.account,
    onChange: e => onAccount(e.target.value),
    options: line.kind === "variance" ? ["Cash Short", "Cash Over", "Manager Adjustment"] : [line.account, "Suspense"]
  }), /*#__PURE__*/React.createElement(MoneyAmount, {
    value: line.amount,
    signed: line.kind === "variance",
    style: {
      width: 92,
      textAlign: "right"
    }
  }));
}
function ShiftReviewScreen({
  data,
  tab,
  onTab,
  selectedId,
  onSelect,
  onAccept,
  accepted,
  lines,
  onLineAccount
}) {
  const rows = data.shifts.filter(s => tab === "all" ? true : s.status === tab);
  const shift = data.shifts.find(s => s.id === selectedId);
  const isAccepted = shift && (accepted.includes(shift.id) || shift.status === "accepted");
  const counts = {
    open: data.shifts.filter(s => s.status === "open").length,
    closed: data.shifts.filter(s => s.status === "closed" && !accepted.includes(s.id)).length,
    accepted: data.shifts.filter(s => s.status === "accepted").length + accepted.length
  };
  return /*#__PURE__*/React.createElement("div", null, /*#__PURE__*/React.createElement(TopBar, {
    breadcrumb: "Service / Shifts",
    title: "Shift review",
    subtitle: counts.closed + (counts.closed === 1 ? " shift" : " shifts") + " awaiting review · closing freezes a count, acceptance posts to GL",
    actions: /*#__PURE__*/React.createElement(React.Fragment, null, /*#__PURE__*/React.createElement(IconButton, {
      icon: "funnel",
      label: "Filter"
    }), /*#__PURE__*/React.createElement(Button, {
      variant: "secondary",
      iconLeft: "download"
    }, "Export GL batch"))
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      padding: "0 var(--gutter-page)"
    }
  }, /*#__PURE__*/React.createElement(Tabs, {
    activeId: tab,
    onSelect: onTab,
    tabs: [{
      id: "open",
      label: "Open",
      count: counts.open
    }, {
      id: "closed",
      label: "Awaiting review",
      count: counts.closed
    }, {
      id: "accepted",
      label: "Accepted",
      count: counts.accepted
    }, {
      id: "cancelled",
      label: "Cancelled"
    }, {
      id: "all",
      label: "All"
    }]
  })), /*#__PURE__*/React.createElement("div", {
    style: {
      padding: "var(--space-7) var(--gutter-page)",
      display: "grid",
      gridTemplateColumns: "1.35fr 1fr",
      gap: "var(--space-6)",
      alignItems: "start",
      maxWidth: "var(--content-max)"
    }
  }, /*#__PURE__*/React.createElement(Card, {
    padding: 0
  }, /*#__PURE__*/React.createElement(DataTable, {
    rows: rows,
    selectedId: selectedId,
    onRowClick: r => onSelect(r.id),
    empty: "No shifts in this state.",
    columns: [{
      key: "id",
      header: "Shift",
      render: r => /*#__PURE__*/React.createElement("span", {
        style: {
          font: "var(--type-numeric)"
        }
      }, r.id)
    }, {
      key: "till",
      header: "Till"
    }, {
      key: "cashier",
      header: "Cashier"
    }, {
      key: "sales",
      header: "Sales",
      align: "right",
      render: r => /*#__PURE__*/React.createElement(MoneyAmount, {
        value: r.sales
      })
    }, {
      key: "variance",
      header: "Variance",
      align: "right",
      render: r => r.variance == null ? /*#__PURE__*/React.createElement("span", {
        style: {
          color: "var(--text-subtle)"
        }
      }, "\u2014") : /*#__PURE__*/React.createElement(MoneyAmount, {
        value: r.variance,
        signed: true
      })
    }, {
      key: "status",
      header: "Status",
      render: r => /*#__PURE__*/React.createElement(StatusPill, {
        status: accepted.includes(r.id) ? "accepted" : r.status
      })
    }]
  })), !shift ? /*#__PURE__*/React.createElement(Card, null, /*#__PURE__*/React.createElement(EmptyState, {
    compact: true,
    icon: "clock",
    title: "Select a shift",
    message: "Pick a shift on the left to review its cash count and proposed GL lines."
  })) : /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gap: "var(--space-6)"
    }
  }, /*#__PURE__*/React.createElement(Card, {
    eyebrow: shift.till + " · " + shift.cashier,
    title: shift.id,
    actions: /*#__PURE__*/React.createElement(StatusPill, {
      status: accepted.includes(shift.id) ? "accepted" : shift.status
    }),
    footer: shift.status === "closed" && !isAccepted ? /*#__PURE__*/React.createElement("div", {
      style: {
        display: "flex",
        justifyContent: "space-between",
        alignItems: "center",
        gap: "var(--space-5)"
      }
    }, /*#__PURE__*/React.createElement("span", {
      style: {
        font: "var(--weight-regular) var(--text-caption)/1.4 var(--font-sans)",
        color: "var(--text-muted)"
      }
    }, "Posting is immutable."), /*#__PURE__*/React.createElement("div", {
      style: {
        display: "flex",
        gap: "var(--gap-inline)"
      }
    }, /*#__PURE__*/React.createElement(Button, {
      variant: "ghost"
    }, "Reopen count"), /*#__PURE__*/React.createElement(Button, {
      variant: "primary",
      iconLeft: "check",
      onClick: onAccept
    }, "Accept & post to GL"))) : null
  }, /*#__PURE__*/React.createElement(KeyValueList, {
    items: [{
      label: "Opened",
      value: /*#__PURE__*/React.createElement("span", {
        className: "aivo-num"
      }, shift.opened)
    }, {
      label: "Closed",
      value: /*#__PURE__*/React.createElement("span", {
        className: "aivo-num"
      }, shift.closed || "—")
    }, {
      label: "Sales",
      value: /*#__PURE__*/React.createElement(MoneyAmount, {
        value: shift.sales
      })
    }, {
      label: "Pay-in / pay-out",
      value: /*#__PURE__*/React.createElement("span", null, /*#__PURE__*/React.createElement(MoneyAmount, {
        value: shift.payIn,
        size: "sm"
      }), " ", /*#__PURE__*/React.createElement("span", {
        style: {
          color: "var(--text-subtle)"
        }
      }, "/"), " ", /*#__PURE__*/React.createElement(MoneyAmount, {
        value: shift.payOut,
        size: "sm"
      }))
    }, {
      label: "Expected cash",
      value: shift.expected == null ? "—" : /*#__PURE__*/React.createElement(MoneyAmount, {
        value: shift.expected
      })
    }, {
      label: "Declared cash",
      value: shift.declared == null ? "—" : /*#__PURE__*/React.createElement(MoneyAmount, {
        value: shift.declared
      })
    }, {
      label: "Variance",
      value: shift.variance == null ? "—" : /*#__PURE__*/React.createElement(MoneyAmount, {
        value: shift.variance,
        signed: true,
        strong: true
      })
    }]
  })), lines ? /*#__PURE__*/React.createElement(Card, {
    eyebrow: isAccepted ? "Posted" : "Proposed",
    title: "General ledger lines"
  }, lines.map((l, i) => /*#__PURE__*/React.createElement(GLLineRow, {
    key: i,
    line: l,
    onAccount: v => onLineAccount(i, v)
  }))) : null, shift.variance ? /*#__PURE__*/React.createElement(AIInsight, {
    compact: true,
    title: "Similar shortages on Till 2 were posted to Manager Adjustment",
    body: "Two of the last three shortages on this till were redirected by Maja R.",
    confidence: 0.48,
    basis: "3 shortages on Till 2 in the last 30 days",
    requiresConfirmation: true,
    acceptLabel: "Redirect line",
    onAccept: () => onLineAccount(lines.length - 1, "Manager Adjustment")
  }) : null)));
}
Object.assign(window, {
  ShiftReviewScreen,
  GLLineRow
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/backoffice/ShiftReviewScreen.jsx", error: String((e && e.message) || e) }); }

// ui_kits/backoffice/TodayScreen.jsx
try { (() => {
const {
  Card,
  StatCard,
  Button,
  IconButton,
  Badge,
  Icon,
  DataTable,
  StatusPill,
  MoneyAmount,
  AIInsight,
  TopBar
} = window.AIVODesignSystem_3d538f;
function ServiceLog({
  entries
}) {
  return /*#__PURE__*/React.createElement("ol", {
    style: {
      listStyle: "none",
      margin: 0,
      padding: 0,
      display: "grid",
      gap: 2
    }
  }, entries.map((e, i) => /*#__PURE__*/React.createElement("li", {
    key: i,
    style: {
      display: "flex",
      gap: 12,
      padding: "9px 0",
      borderBottom: i < entries.length - 1 ? "1px dashed var(--border-default)" : "none"
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-regular) var(--text-caption)/1.5 var(--font-mono)",
      color: "var(--text-subtle)",
      flex: "none",
      width: 40
    }
  }, e.time), e.tone === "ai" ? /*#__PURE__*/React.createElement(Icon, {
    name: "sparkles",
    size: 14,
    style: {
      color: "var(--ai-marker)",
      marginTop: 2
    }
  }) : null, /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-regular) var(--text-body-sm)/1.5 var(--font-sans)",
      color: e.tone === "warn" ? "var(--red-700)" : "var(--text-body)"
    }
  }, e.text))));
}
function TodayScreen({
  data,
  onGoShifts,
  onDismissInsight,
  insightVisible
}) {
  const open = data.shifts.filter(s => s.status === "open");
  const awaiting = data.shifts.filter(s => s.status === "closed");
  return /*#__PURE__*/React.createElement("div", null, /*#__PURE__*/React.createElement(TopBar, {
    breadcrumb: "Service",
    title: "Tuesday, 19 August",
    subtitle: "Dinner service \xB7 128 covers so far \xB7 3 tills configured",
    actions: /*#__PURE__*/React.createElement(React.Fragment, null, /*#__PURE__*/React.createElement(IconButton, {
      icon: "bell",
      label: "Alerts"
    }), /*#__PURE__*/React.createElement(Button, {
      variant: "secondary",
      iconLeft: "download"
    }, "Day report"), /*#__PURE__*/React.createElement(Button, {
      variant: "primary",
      iconLeft: "plus"
    }, "Open shift"))
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      padding: "var(--space-8) var(--gutter-page)",
      display: "grid",
      gap: "var(--gap-section)",
      maxWidth: "var(--content-max)"
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gridTemplateColumns: "repeat(4,1fr)",
      gap: "var(--space-6)"
    }
  }, /*#__PURE__*/React.createElement(StatCard, {
    label: "Net sales",
    value: "$4,355",
    delta: "+8.4%",
    caption: "vs. last Tuesday",
    icon: "banknote"
  }), /*#__PURE__*/React.createElement(StatCard, {
    label: "Covers",
    value: "128",
    delta: "+12",
    caption: "forecast was 116",
    icon: "users"
  }), /*#__PURE__*/React.createElement(StatCard, {
    label: "Avg. ticket",
    value: "$34.02",
    delta: "-1.2%",
    deltaDirection: "down",
    icon: "receipt"
  }), /*#__PURE__*/React.createElement(StatCard, {
    tone: "inverse",
    label: "Awaiting review",
    value: awaiting.length,
    unit: "shifts",
    caption: "Total variance -$5.00"
  })), /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gridTemplateColumns: "1.5fr 1fr",
      gap: "var(--space-6)",
      alignItems: "start"
    }
  }, /*#__PURE__*/React.createElement(Card, {
    title: "Open shifts",
    eyebrow: "Live",
    padding: 0,
    actions: /*#__PURE__*/React.createElement(Button, {
      size: "sm",
      variant: "ghost",
      iconRight: "arrow-right",
      onClick: onGoShifts
    }, "All shifts")
  }, /*#__PURE__*/React.createElement(DataTable, {
    rows: open.concat(awaiting),
    columns: [{
      key: "id",
      header: "Shift",
      render: r => /*#__PURE__*/React.createElement("span", {
        style: {
          font: "var(--type-numeric)"
        }
      }, r.id)
    }, {
      key: "till",
      header: "Till"
    }, {
      key: "cashier",
      header: "Cashier"
    }, {
      key: "opened",
      header: "Opened",
      align: "right",
      render: r => /*#__PURE__*/React.createElement("span", {
        style: {
          font: "var(--type-numeric)",
          color: "var(--text-muted)"
        }
      }, r.opened)
    }, {
      key: "sales",
      header: "Sales",
      align: "right",
      render: r => /*#__PURE__*/React.createElement(MoneyAmount, {
        value: r.sales
      })
    }, {
      key: "status",
      header: "Status",
      render: r => /*#__PURE__*/React.createElement(StatusPill, {
        status: r.status
      })
    }],
    onRowClick: onGoShifts
  })), /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gap: "var(--space-6)"
    }
  }, insightVisible ? /*#__PURE__*/React.createElement(AIInsight, {
    title: "Prep 18 more lamb portions for tomorrow",
    body: "Tuesday covers have run 12% above forecast for three weeks and the lamb sold out before 20:30 twice.",
    confidence: 0.82,
    basis: "8 weeks of Tuesday covers and last week's sell-out times",
    acceptLabel: "Add to prep list",
    onAccept: onDismissInsight,
    onDismiss: onDismissInsight
  }) : null, /*#__PURE__*/React.createElement(Card, {
    title: "Service log",
    eyebrow: "Today"
  }, /*#__PURE__*/React.createElement(ServiceLog, {
    entries: data.service
  })))), /*#__PURE__*/React.createElement(Card, {
    title: "Open orders",
    eyebrow: "Floor",
    padding: 0,
    actions: /*#__PURE__*/React.createElement(Badge, {
      tone: "neutral"
    }, data.orders.filter(o => o.state === "open").length, " open")
  }, /*#__PURE__*/React.createElement(DataTable, {
    dense: true,
    rows: data.orders,
    columns: [{
      key: "id",
      header: "Order",
      render: r => /*#__PURE__*/React.createElement("span", {
        style: {
          font: "var(--type-numeric)"
        }
      }, r.id)
    }, {
      key: "table",
      header: "Table"
    }, {
      key: "waiter",
      header: "Waiter"
    }, {
      key: "covers",
      header: "Covers",
      align: "right"
    }, {
      key: "items",
      header: "Items",
      align: "right"
    }, {
      key: "opened",
      header: "Opened",
      align: "right",
      render: r => /*#__PURE__*/React.createElement("span", {
        style: {
          font: "var(--type-numeric)",
          color: "var(--text-muted)"
        }
      }, r.opened)
    }, {
      key: "total",
      header: "Total",
      align: "right",
      render: r => /*#__PURE__*/React.createElement(MoneyAmount, {
        value: r.total
      })
    }, {
      key: "state",
      header: "Status",
      render: r => /*#__PURE__*/React.createElement(StatusPill, {
        status: r.state
      })
    }]
  }))));
}
Object.assign(window, {
  TodayScreen,
  ServiceLog
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/backoffice/TodayScreen.jsx", error: String((e && e.message) || e) }); }

// ui_kits/backoffice/data.js
try { (() => {
const shifts = [{
  id: "shift-118",
  till: "Till 1",
  cashier: "Alice Toma",
  status: "open",
  opened: "17:02",
  sales: 1412.75,
  payIn: 200,
  payOut: 40,
  expected: null,
  declared: null,
  variance: null
}, {
  id: "shift-117",
  till: "Till 2",
  cashier: "Bob Nagy",
  status: "closed",
  opened: "11:58",
  closed: "17:04",
  sales: 984.5,
  payIn: 0,
  payOut: 60,
  expected: 924.5,
  declared: 919.5,
  variance: -5
}, {
  id: "shift-116",
  till: "Terrace",
  cashier: "Carol Ilie",
  status: "closed",
  opened: "11:40",
  closed: "16:52",
  sales: 512,
  payIn: 50,
  payOut: 0,
  expected: 562,
  declared: 562,
  variance: 0
}, {
  id: "shift-115",
  till: "Till 1",
  cashier: "Dave Marin",
  status: "accepted",
  opened: "07:55",
  closed: "11:50",
  sales: 806.25,
  payIn: 0,
  payOut: 25,
  expected: 781.25,
  declared: 783.25,
  variance: 2
}, {
  id: "shift-114",
  till: "Till 2",
  cashier: "Erin Voda",
  status: "accepted",
  opened: "07:50",
  closed: "11:45",
  sales: 640,
  payIn: 0,
  payOut: 0,
  expected: 640,
  declared: 640,
  variance: 0
}, {
  id: "shift-113",
  till: "Terrace",
  cashier: "Frank Popa",
  status: "cancelled",
  opened: "07:48",
  closed: null,
  sales: 0,
  payIn: 0,
  payOut: 0,
  expected: null,
  declared: null,
  variance: null
}];
const glLines = {
  "shift-117": [{
    kind: "sales",
    label: "Sales revenue",
    account: "Sales Revenue",
    amount: 984.5,
    locked: false
  }, {
    kind: "payout",
    label: "Pay-out — petty cash",
    account: "Cash Clearing",
    amount: -60,
    locked: false
  }, {
    kind: "variance",
    label: "Cash short",
    account: "Cash Short",
    amount: -5,
    locked: true
  }],
  "shift-116": [{
    kind: "sales",
    label: "Sales revenue",
    account: "Sales Revenue",
    amount: 512,
    locked: false
  }, {
    kind: "payin",
    label: "Pay-in — float top-up",
    account: "Cash Clearing",
    amount: 50,
    locked: false
  }, {
    kind: "variance",
    label: "Cash variance (none)",
    account: "Cash Short",
    amount: 0,
    locked: true
  }]
};
const orders = [{
  id: "#2841",
  table: "T4",
  covers: 2,
  items: 6,
  total: 78.5,
  state: "open",
  waiter: "Ines",
  opened: "19:12"
}, {
  id: "#2840",
  table: "T9",
  covers: 4,
  items: 11,
  total: 164.0,
  state: "open",
  waiter: "Ines",
  opened: "19:04"
}, {
  id: "#2839",
  table: "Bar 2",
  covers: 1,
  items: 3,
  total: 31.5,
  state: "closed",
  waiter: "Luca",
  opened: "18:51"
}, {
  id: "#2838",
  table: "T2",
  covers: 3,
  items: 9,
  total: 121.25,
  state: "closed",
  waiter: "Maja",
  opened: "18:33"
}, {
  id: "#2837",
  table: "Takeaway",
  covers: 1,
  items: 2,
  total: 24.0,
  state: "accepted",
  waiter: "POS",
  opened: "18:20"
}];
const menu = [{
  id: "m1",
  name: "Lamb shoulder, 8h",
  category: "Mains",
  price: 26,
  cost: 8.4,
  sold: 41,
  stock: 18,
  flag: "Sold out twice last week"
}, {
  id: "m2",
  name: "Cacio e pepe",
  category: "Mains",
  price: 17,
  cost: 3.1,
  sold: 63,
  stock: null,
  flag: null
}, {
  id: "m3",
  name: "Focaccia, rosemary",
  category: "Starters",
  price: 7,
  cost: 1.2,
  sold: 88,
  stock: null,
  flag: null
}, {
  id: "m4",
  name: "Burrata & peach",
  category: "Starters",
  price: 14,
  cost: 5.9,
  sold: 22,
  stock: 9,
  flag: "Margin below target"
}, {
  id: "m5",
  name: "Tiramisu",
  category: "Desserts",
  price: 9,
  cost: 2.0,
  sold: 37,
  stock: null,
  flag: null
}, {
  id: "m6",
  name: "House red, glass",
  category: "Drinks",
  price: 8,
  cost: 1.8,
  sold: 104,
  stock: null,
  flag: null
}];
const service = [{
  time: "19:12",
  text: "Order #2841 opened on T4 by Ines",
  tone: "neutral"
}, {
  time: "18:58",
  text: "AIVO flagged lamb shoulder — 18 portions left, pace suggests sell-out by 20:40",
  tone: "ai"
}, {
  time: "17:04",
  text: "shift-117 closed by Bob Nagy — variance -$5.00",
  tone: "warn"
}, {
  time: "16:52",
  text: "shift-116 closed by Carol Ilie — variance $0.00",
  tone: "good"
}, {
  time: "11:50",
  text: "shift-115 accepted by Maja R. — 3 GL lines posted",
  tone: "good"
}];
Object.assign(window, {
  AIVO_DATA: {
    shifts,
    glLines,
    orders,
    menu,
    service
  }
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/backoffice/data.js", error: String((e && e.message) || e) }); }

// ui_kits/pos/CloseShiftScreen.jsx
try { (() => {
const {
  Card,
  Button,
  Input,
  KeyValueList,
  MoneyAmount,
  Icon,
  Checkbox,
  StatusPill
} = window.AIVODesignSystem_3d538f;
function CloseShiftScreen({
  shift,
  expected,
  onBack,
  onClose
}) {
  const [declared, setDeclared] = React.useState(expected.toFixed(2));
  const [counted, setCounted] = React.useState(false);
  const variance = Math.round((parseFloat(declared || 0) - expected) * 100) / 100;
  return /*#__PURE__*/React.createElement("div", {
    style: {
      height: "100%",
      display: "grid",
      placeItems: "center",
      background: "var(--paper-2)",
      padding: "var(--space-8)"
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      width: 620,
      display: "grid",
      gap: "var(--space-6)"
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: "flex",
      alignItems: "center",
      gap: "var(--space-5)"
    }
  }, /*#__PURE__*/React.createElement(Button, {
    variant: "ghost",
    iconLeft: "arrow-right",
    onClick: onBack,
    style: {
      transform: "scaleX(1)"
    }
  }, "Back to order"), /*#__PURE__*/React.createElement(StatusPill, {
    status: "open",
    label: shift.till + " · " + shift.cashier
  })), /*#__PURE__*/React.createElement(Card, {
    eyebrow: "Closing uses the current moment \u2014 no backdating",
    title: "Count the drawer"
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gap: "var(--space-7)"
    }
  }, /*#__PURE__*/React.createElement(KeyValueList, {
    items: [{
      label: "Sales rung this shift",
      value: /*#__PURE__*/React.createElement(MoneyAmount, {
        value: expected - 200 + 0
      })
    }, {
      label: "Opening float",
      value: /*#__PURE__*/React.createElement(MoneyAmount, {
        value: 200
      })
    }, {
      label: "Expected cash",
      value: /*#__PURE__*/React.createElement(MoneyAmount, {
        value: expected,
        strong: true
      })
    }]
  }), /*#__PURE__*/React.createElement(Input, {
    label: "Declared cash",
    type: "number",
    prefix: "$",
    size: "lg",
    value: declared,
    onChange: e => setDeclared(e.target.value),
    hint: "What you physically counted in the drawer"
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      display: "flex",
      alignItems: "center",
      justifyContent: "space-between",
      padding: "var(--space-6)",
      borderRadius: "var(--radius-sm)",
      background: variance === 0 ? "var(--green-100)" : "var(--red-50)",
      border: "1px solid " + (variance === 0 ? "var(--green-100)" : "var(--red-200)")
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      display: "flex",
      alignItems: "center",
      gap: 8,
      font: "var(--type-label)",
      color: variance === 0 ? "var(--green-700)" : "var(--red-800)"
    }
  }, /*#__PURE__*/React.createElement(Icon, {
    name: variance === 0 ? "circle-check" : "triangle-alert",
    size: 16
  }), variance === 0 ? "No variance" : variance < 0 ? "Cash short — posts to Cash Short on acceptance" : "Cash over — posts to Cash Over on acceptance"), /*#__PURE__*/React.createElement(MoneyAmount, {
    value: variance,
    signed: true,
    size: "lg",
    strong: true
  })), /*#__PURE__*/React.createElement(Checkbox, {
    label: "I counted the drawer myself",
    description: "Required \u2014 the count is frozen at close and reviewed in the back office.",
    checked: counted,
    onChange: e => setCounted(e.target.checked)
  }), /*#__PURE__*/React.createElement(Button, {
    size: "touch",
    variant: "primary",
    fullWidth: true,
    iconLeft: "lock",
    disabled: !counted,
    onClick: () => onClose(variance)
  }, "Close shift & freeze count"), /*#__PURE__*/React.createElement("p", {
    style: {
      font: "var(--weight-regular) var(--text-caption)/1.5 var(--font-sans)",
      color: "var(--text-muted)"
    }
  }, "Closing does not post to the ledger. A manager accepts the shift in the back office, and only then does anything hit GL.")))));
}
Object.assign(window, {
  CloseShiftScreen
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/pos/CloseShiftScreen.jsx", error: String((e && e.message) || e) }); }

// ui_kits/pos/OpenShiftScreen.jsx
try { (() => {
const {
  Card,
  Button,
  Select,
  Input,
  Icon,
  Badge,
  StatusPill
} = window.AIVODesignSystem_3d538f;
function OpenShiftScreen({
  onOpen,
  conflict
}) {
  const [till, setTill] = React.useState("Till 1");
  const [cashier, setCashier] = React.useState("Alice Toma");
  return /*#__PURE__*/React.createElement("div", {
    style: {
      height: "100%",
      display: "grid",
      placeItems: "center",
      background: "var(--paper-2)"
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      width: 460,
      display: "grid",
      gap: "var(--space-8)"
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gap: 6,
      justifyItems: "center"
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-semibold) 30px/1 var(--font-sans)",
      letterSpacing: "-0.035em",
      color: "var(--ink-900)"
    }
  }, "aivo", /*#__PURE__*/React.createElement("span", {
    style: {
      color: "var(--accent-solid)"
    }
  }, ".")), /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--type-label)",
      color: "var(--text-muted)"
    }
  }, "POS terminal \xB7 Osteria Nord")), /*#__PURE__*/React.createElement(Card, {
    title: "Open a shift",
    eyebrow: "One open shift per till and per cashier"
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gap: "var(--space-6)"
    }
  }, /*#__PURE__*/React.createElement(Select, {
    label: "Till",
    value: till,
    onChange: e => setTill(e.target.value),
    size: "lg",
    options: ["Till 1", "Till 2", "Terrace"]
  }), /*#__PURE__*/React.createElement(Select, {
    label: "Cashier",
    value: cashier,
    onChange: e => setCashier(e.target.value),
    size: "lg",
    options: ["Alice Toma", "Bob Nagy", "Carol Ilie"]
  }), /*#__PURE__*/React.createElement(Input, {
    label: "Opening float",
    type: "number",
    prefix: "$",
    defaultValue: "200",
    size: "lg",
    hint: "Counted into the drawer now"
  }), conflict ? /*#__PURE__*/React.createElement("div", {
    style: {
      display: "flex",
      gap: 8,
      alignItems: "flex-start",
      padding: "var(--space-5)",
      background: "var(--red-50)",
      border: "1px solid var(--red-200)",
      borderRadius: "var(--radius-sm)"
    }
  }, /*#__PURE__*/React.createElement(Icon, {
    name: "triangle-alert",
    size: 16,
    style: {
      color: "var(--red-600)",
      marginTop: 1
    }
  }), /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-regular) var(--text-body-sm)/1.5 var(--font-sans)",
      color: "var(--red-800)"
    }
  }, conflict)) : null, /*#__PURE__*/React.createElement(Button, {
    size: "touch",
    variant: "primary",
    fullWidth: true,
    iconLeft: "lock",
    onClick: () => onOpen({
      till,
      cashier
    })
  }, "Open shift"))), /*#__PURE__*/React.createElement("div", {
    style: {
      display: "flex",
      gap: 10,
      justifyContent: "center",
      alignItems: "center"
    }
  }, /*#__PURE__*/React.createElement(StatusPill, {
    status: "open",
    label: "Till 2 \xB7 Bob Nagy"
  }), /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-regular) var(--text-caption)/1.4 var(--font-sans)",
      color: "var(--text-muted)"
    }
  }, "already ringing since 11:58"))));
}
Object.assign(window, {
  OpenShiftScreen
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/pos/OpenShiftScreen.jsx", error: String((e && e.message) || e) }); }

// ui_kits/pos/OrderScreen.jsx
try { (() => {
const {
  Button,
  IconButton,
  Icon,
  Badge,
  MoneyAmount,
  QuantityStepper,
  StatusPill,
  Tabs,
  EmptyState,
  AIInsight
} = window.AIVODesignSystem_3d538f;
function ProductTile({
  item,
  onAdd
}) {
  const [press, setPress] = React.useState(false);
  return /*#__PURE__*/React.createElement("button", {
    type: "button",
    onClick: () => onAdd(item),
    onMouseDown: () => setPress(true),
    onMouseUp: () => setPress(false),
    onMouseLeave: () => setPress(false),
    style: {
      display: "grid",
      gap: 8,
      alignContent: "space-between",
      textAlign: "left",
      height: 108,
      padding: "var(--space-6)",
      background: "var(--surface-card)",
      border: "1px solid var(--border-default)",
      borderRadius: "var(--radius-md)",
      boxShadow: press ? "var(--shadow-press)" : "var(--shadow-sm)",
      transform: press ? "translateY(1px)" : "none",
      cursor: "pointer",
      transition: "box-shadow var(--dur-fast) var(--ease-out)"
    }
  }, /*#__PURE__*/React.createElement(Icon, {
    name: item.icon,
    size: 18,
    style: {
      color: "var(--text-subtle)"
    }
  }), /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-medium) var(--text-body-lg)/1.25 var(--font-sans)",
      color: "var(--text-strong)"
    }
  }, item.name), /*#__PURE__*/React.createElement(MoneyAmount, {
    value: item.price,
    tone: "muted"
  }));
}
function Ticket({
  lines,
  table,
  onQty,
  onSend,
  onTender,
  total
}) {
  return /*#__PURE__*/React.createElement("aside", {
    style: {
      width: 380,
      flex: "none",
      display: "grid",
      gridTemplateRows: "auto 1fr auto",
      background: "var(--surface-card)",
      borderLeft: "1px solid var(--border-default)"
    }
  }, /*#__PURE__*/React.createElement("header", {
    style: {
      padding: "var(--space-6) var(--space-7)",
      borderBottom: "1px solid var(--border-subtle)",
      display: "flex",
      alignItems: "center",
      justifyContent: "space-between"
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gap: 3
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--type-eyebrow)",
      letterSpacing: "var(--tracking-caps)",
      textTransform: "uppercase",
      color: "var(--text-muted)"
    }
  }, "Order #2842"), /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-regular) var(--text-title-lg)/1.2 var(--font-display)",
      color: "var(--text-strong)"
    }
  }, table)), /*#__PURE__*/React.createElement(StatusPill, {
    status: "open"
  })), /*#__PURE__*/React.createElement("div", {
    style: {
      overflowY: "auto",
      padding: "var(--space-5) var(--space-7)"
    }
  }, lines.length === 0 ? /*#__PURE__*/React.createElement(EmptyState, {
    compact: true,
    icon: "receipt",
    title: "Empty ticket",
    message: "Tap a product to start the order."
  }) : lines.map(l => /*#__PURE__*/React.createElement("div", {
    key: l.id,
    style: {
      display: "flex",
      alignItems: "center",
      gap: "var(--space-5)",
      padding: "10px 0",
      borderBottom: "1px dashed var(--border-default)"
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      flex: 1,
      display: "grid",
      gap: 2
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--type-label)",
      color: "var(--text-strong)"
    }
  }, l.name), /*#__PURE__*/React.createElement(MoneyAmount, {
    value: l.price,
    size: "sm",
    tone: "muted"
  })), /*#__PURE__*/React.createElement(QuantityStepper, {
    value: l.qty,
    onChange: n => onQty(l.id, n)
  }), /*#__PURE__*/React.createElement(MoneyAmount, {
    value: l.price * l.qty,
    style: {
      width: 76,
      textAlign: "right"
    }
  })))), /*#__PURE__*/React.createElement("footer", {
    style: {
      padding: "var(--space-6) var(--space-7)",
      borderTop: "1px solid var(--border-default)",
      background: "var(--paper-1)",
      display: "grid",
      gap: "var(--space-5)"
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gap: 6
    }
  }, /*#__PURE__*/React.createElement(Row, {
    label: "Subtotal",
    value: total
  }), /*#__PURE__*/React.createElement(Row, {
    label: "Service 0%",
    value: 0,
    muted: true
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      display: "flex",
      justifyContent: "space-between",
      alignItems: "baseline",
      paddingTop: 8,
      borderTop: "1px solid var(--border-default)"
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-semibold) var(--text-title-sm)/1 var(--font-sans)",
      color: "var(--text-strong)"
    }
  }, "Total"), /*#__PURE__*/React.createElement(MoneyAmount, {
    value: total,
    size: "lg",
    strong: true
  }))), /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gridTemplateColumns: "1fr 1fr",
      gap: "var(--gap-inline)"
    }
  }, /*#__PURE__*/React.createElement(Button, {
    size: "touch",
    variant: "secondary",
    iconLeft: "printer",
    onClick: onSend
  }, "Send to kitchen"), /*#__PURE__*/React.createElement(Button, {
    size: "touch",
    variant: "primary",
    iconLeft: "banknote",
    onClick: onTender,
    disabled: lines.length === 0
  }, "Tender"))));
}
function Row({
  label,
  value,
  muted
}) {
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: "flex",
      justifyContent: "space-between",
      font: "var(--type-body)",
      color: "var(--text-muted)"
    }
  }, /*#__PURE__*/React.createElement("span", null, label), /*#__PURE__*/React.createElement(MoneyAmount, {
    value: value,
    tone: muted ? "muted" : "neutral",
    size: "sm"
  }));
}
function OrderScreen({
  shift,
  lines,
  onAdd,
  onQty,
  onSend,
  onTender,
  onCloseShift,
  table,
  onTable,
  tables,
  menu,
  banner
}) {
  const [cat, setCat] = React.useState("Mains");
  const cats = Array.from(new Set(menu.map(m => m.cat)));
  const total = lines.reduce((a, l) => a + l.price * l.qty, 0);
  return /*#__PURE__*/React.createElement("div", {
    style: {
      height: "100%",
      display: "flex",
      background: "var(--paper-2)"
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1,
      display: "grid",
      gridTemplateRows: "auto auto 1fr",
      minWidth: 0
    }
  }, /*#__PURE__*/React.createElement("header", {
    style: {
      display: "flex",
      alignItems: "center",
      gap: "var(--space-6)",
      padding: "var(--space-5) var(--space-8)",
      background: "var(--paper-1)",
      borderBottom: "1px solid var(--border-default)"
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      font: "var(--weight-semibold) var(--text-title-md)/1 var(--font-sans)",
      letterSpacing: "-0.03em",
      color: "var(--ink-900)"
    }
  }, "aivo", /*#__PURE__*/React.createElement("span", {
    style: {
      color: "var(--accent-solid)"
    }
  }, ".")), /*#__PURE__*/React.createElement("span", {
    style: {
      display: "flex",
      alignItems: "center",
      gap: 8,
      font: "var(--type-label)",
      color: "var(--text-muted)"
    }
  }, /*#__PURE__*/React.createElement(StatusPill, {
    status: "open",
    label: shift.till
  }), " ", shift.cashier), /*#__PURE__*/React.createElement("div", {
    style: {
      marginLeft: "auto",
      display: "flex",
      alignItems: "center",
      gap: "var(--gap-inline)"
    }
  }, /*#__PURE__*/React.createElement(IconButton, {
    icon: "search",
    label: "Find order",
    size: "lg"
  }), /*#__PURE__*/React.createElement(IconButton, {
    icon: "bell",
    label: "Kitchen calls",
    size: "lg"
  }), /*#__PURE__*/React.createElement(Button, {
    variant: "secondary",
    iconLeft: "calculator",
    onClick: onCloseShift
  }, "Close shift"))), /*#__PURE__*/React.createElement("div", {
    style: {
      padding: "var(--space-5) var(--space-8) 0",
      display: "flex",
      gap: "var(--space-8)",
      alignItems: "center",
      flexWrap: "wrap"
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: "flex",
      gap: 6,
      flexWrap: "wrap"
    }
  }, tables.map(t => /*#__PURE__*/React.createElement("button", {
    key: t,
    type: "button",
    onClick: () => onTable(t),
    style: {
      height: 38,
      padding: "0 14px",
      borderRadius: "var(--radius-pill)",
      cursor: "pointer",
      background: t === table ? "var(--ink-900)" : "var(--surface-card)",
      color: t === table ? "var(--paper-1)" : "var(--text-body)",
      border: "1px solid " + (t === table ? "var(--ink-900)" : "var(--border-default)"),
      font: "var(--type-label)"
    }
  }, t)))), /*#__PURE__*/React.createElement("div", {
    style: {
      padding: "var(--space-6) var(--space-8) var(--space-8)",
      overflowY: "auto",
      display: "grid",
      gap: "var(--space-6)",
      alignContent: "start"
    }
  }, banner ? /*#__PURE__*/React.createElement(AIInsight, {
    compact: true,
    title: banner,
    confidence: 0.79,
    basis: "tonight's pace against 8 weeks of Tuesdays",
    acceptLabel: "Mark 86",
    onAccept: () => {},
    onDismiss: () => {}
  }) : null, /*#__PURE__*/React.createElement(Tabs, {
    activeId: cat,
    onSelect: setCat,
    tabs: cats.map(c => ({
      id: c,
      label: c
    }))
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      display: "grid",
      gridTemplateColumns: "repeat(auto-fill,minmax(180px,1fr))",
      gap: "var(--space-5)"
    }
  }, menu.filter(m => m.cat === cat).map(m => /*#__PURE__*/React.createElement(ProductTile, {
    key: m.id,
    item: m,
    onAdd: onAdd
  }))))), /*#__PURE__*/React.createElement(Ticket, {
    lines: lines,
    table: table,
    onQty: onQty,
    onSend: onSend,
    onTender: onTender,
    total: total
  }));
}
Object.assign(window, {
  OrderScreen,
  ProductTile,
  Ticket
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/pos/OrderScreen.jsx", error: String((e && e.message) || e) }); }

// ui_kits/pos/data.js
try { (() => {
const posMenu = [{
  id: "p1",
  name: "Focaccia",
  cat: "Starters",
  price: 7,
  icon: "utensils"
}, {
  id: "p2",
  name: "Burrata & peach",
  cat: "Starters",
  price: 14,
  icon: "utensils"
}, {
  id: "p3",
  name: "Cacio e pepe",
  cat: "Mains",
  price: 17,
  icon: "hand-platter"
}, {
  id: "p4",
  name: "Lamb shoulder",
  cat: "Mains",
  price: 26,
  icon: "hand-platter"
}, {
  id: "p5",
  name: "Sea bream",
  cat: "Mains",
  price: 24,
  icon: "hand-platter"
}, {
  id: "p6",
  name: "Tiramisu",
  cat: "Desserts",
  price: 9,
  icon: "coffee"
}, {
  id: "p7",
  name: "Affogato",
  cat: "Desserts",
  price: 8,
  icon: "coffee"
}, {
  id: "p8",
  name: "House red",
  cat: "Drinks",
  price: 8,
  icon: "banknote"
}, {
  id: "p9",
  name: "Negroni",
  cat: "Drinks",
  price: 12,
  icon: "banknote"
}, {
  id: "p10",
  name: "Sparkling water",
  cat: "Drinks",
  price: 4,
  icon: "banknote"
}, {
  id: "p11",
  name: "Espresso",
  cat: "Drinks",
  price: 3,
  icon: "coffee"
}, {
  id: "p12",
  name: "Takeaway box",
  cat: "Extras",
  price: 1,
  icon: "receipt"
}];
const posTables = ["T1", "T2", "T4", "T9", "Bar 1", "Bar 2", "Terrace 3", "Takeaway"];
Object.assign(window, {
  POS_DATA: {
    posMenu,
    posTables
  }
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/pos/data.js", error: String((e && e.message) || e) }); }

__ds_ns.Badge = __ds_scope.Badge;

__ds_ns.Button = __ds_scope.Button;

__ds_ns.Card = __ds_scope.Card;

__ds_ns.Icon = __ds_scope.Icon;

__ds_ns.IconButton = __ds_scope.IconButton;

__ds_ns.StatCard = __ds_scope.StatCard;

__ds_ns.DataTable = __ds_scope.DataTable;

__ds_ns.KeyValueList = __ds_scope.KeyValueList;

__ds_ns.MoneyAmount = __ds_scope.MoneyAmount;

__ds_ns.StatusPill = __ds_scope.StatusPill;

__ds_ns.AIInsight = __ds_scope.AIInsight;

__ds_ns.Dialog = __ds_scope.Dialog;

__ds_ns.EmptyState = __ds_scope.EmptyState;

__ds_ns.Toast = __ds_scope.Toast;

__ds_ns.Tooltip = __ds_scope.Tooltip;

__ds_ns.Checkbox = __ds_scope.Checkbox;

__ds_ns.Input = __ds_scope.Input;

__ds_ns.QuantityStepper = __ds_scope.QuantityStepper;

__ds_ns.Select = __ds_scope.Select;

__ds_ns.Switch = __ds_scope.Switch;

__ds_ns.SidebarNav = __ds_scope.SidebarNav;

__ds_ns.Tabs = __ds_scope.Tabs;

__ds_ns.TopBar = __ds_scope.TopBar;

})();

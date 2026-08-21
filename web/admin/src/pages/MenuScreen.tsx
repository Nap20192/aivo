import { NavLink } from "react-router-dom";
import Design from "./Design";
import { ItemsTab } from "./Menu";

const SUBS = {
  items:
    "Categories, items, options. Changes go live on the diner menu immediately.",
  design:
    "What diners see when they scan the QR. The preview updates as you edit — nothing goes live until you save.",
  brief:
    "The restaurant's design brief. Saved with the theme; AIVO can propose a theme from it.",
} as const;

export default function MenuScreen(props: {
  tab: "items" | "design" | "brief";
}) {
  return (
    <div className="content">
      <div className="page-head">
        <div>
          <h1 className="page-title">Menu</h1>
          <p className="page-sub">{SUBS[props.tab]}</p>
        </div>
      </div>
      <div className="tabs" style={{ marginBottom: "var(--gap-section)" }}>
        <NavLink
          to="/menu"
          end
          className={({ isActive }) => "tab" + (isActive ? " on" : "")}
        >
          Items
        </NavLink>
        <NavLink
          to="/menu/design"
          className={({ isActive }) => "tab" + (isActive ? " on" : "")}
        >
          Design
        </NavLink>
        <NavLink
          to="/menu/brief"
          className={({ isActive }) => "tab" + (isActive ? " on" : "")}
        >
          design.md
        </NavLink>
      </div>
      {props.tab === "items" ? (
        <ItemsTab />
      ) : (
        <Design tab={props.tab === "design" ? "theme" : "design_md"} />
      )}
    </div>
  );
}

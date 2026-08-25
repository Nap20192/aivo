import { useState } from "react";
import { api } from "../api/client";
import { useRestaurant } from "../auth";
import { useLoad } from "../lib/useLoad";
import { ErrorBanner, LoadingPage } from "../ui";
import Products from "./inventory/Products";
import Receipts from "./inventory/Receipts";
import { FoodCost, StockLevels } from "./inventory/Stock";
import Stocktakes from "./inventory/Stocktakes";
import Suppliers from "./inventory/Suppliers";
import TechCards from "./inventory/TechCards";
import WriteOffs from "./inventory/WriteOffs";

type Tab =
  | "stock"
  | "products"
  | "tech-cards"
  | "receipts"
  | "write-offs"
  | "stocktakes"
  | "food-cost"
  | "suppliers";

const TABS: { id: Tab; label: string }[] = [
  { id: "stock", label: "Stock" },
  { id: "products", label: "Nomenclature" },
  { id: "tech-cards", label: "Tech-cards" },
  { id: "receipts", label: "Receipts" },
  { id: "write-offs", label: "Write-offs" },
  { id: "stocktakes", label: "Stocktakes" },
  { id: "food-cost", label: "Food cost" },
  { id: "suppliers", label: "Suppliers" },
];

const SUBS: Record<Tab, string> = {
  stock: "On-hand quantities at weighted-average cost, and the append-only book of stock movements.",
  products: "Goods, prepared items, dishes and modifiers — each in a fixed base unit, dishes linked to a menu item.",
  "tech-cards": "Calendar-versioned recipes: each version is valid over a date interval; costing is an append-only series.",
  receipts: "Deliveries from suppliers. Posting moves stock in and books Inventory / Accounts payable.",
  "write-offs": "Spoilage, staff meals and losses. Posting depletes stock at the current average.",
  stocktakes: "Physical counts. Dry-run previews variances without saving; posting books surpluses and shortages.",
  "food-cost": "Actual vs theoretical cost of goods sold per menu item, and the share of sales costed off negative stock.",
  suppliers: "The vendors you receive goods from.",
};

export default function Inventory() {
  const [tab, setTab] = useState<Tab>("stock");
  // Selected product for the tech-cards tab (set when opened from Nomenclature).
  const [cardProduct, setCardProduct] = useState("");

  return (
    <div className="content">
      <div className="page-head">
        <div>
          <h1 className="page-title">Inventory</h1>
          <p className="page-sub">{SUBS[tab]}</p>
        </div>
      </div>

      <div className="tabs" style={{ marginBottom: "var(--gap-section)", flexWrap: "wrap" }}>
        {TABS.map((t) => (
          <button key={t.id} className={"tab" + (tab === t.id ? " on" : "")} onClick={() => setTab(t.id)}>
            {t.label}
          </button>
        ))}
      </div>

      {tab === "stock" && <StockLevels />}
      {tab === "products" && (
        <Products
          onOpenCard={(pid) => {
            setCardProduct(pid);
            setTab("tech-cards");
          }}
        />
      )}
      {tab === "tech-cards" && <TechCardsTab selected={cardProduct} setSelected={setCardProduct} />}
      {tab === "receipts" && <Receipts />}
      {tab === "write-offs" && <WriteOffs />}
      {tab === "stocktakes" && <Stocktakes />}
      {tab === "food-cost" && <FoodCost />}
      {tab === "suppliers" && <Suppliers />}
    </div>
  );
}

// Tech-cards needs the product list for the picker + ingredient editor.
function TechCardsTab({ selected, setSelected }: { selected: string; setSelected: (id: string) => void }) {
  const r = useRestaurant();
  const { data, error, loading, reload } = useLoad(() => api.listProducts(r.id), [r.id]);
  if (error) return <ErrorBanner message={error} onRetry={reload} />;
  if (loading || !data) return <LoadingPage />;
  return <TechCards products={data} selected={selected} setSelected={setSelected} />;
}

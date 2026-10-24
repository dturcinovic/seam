// A fixture.
type Props = { name: string };

export const App = ({ name }: Props) => (
  <section id="main">
    <h1 className="title">{name.length > 3 ? name : "short"}</h1>
  </section>
);

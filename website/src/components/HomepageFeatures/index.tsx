import clsx from 'clsx';
import Heading from '@theme/Heading';
import styles from './styles.module.css';

type FeatureItem = {
  title: string;
  Svg: React.ComponentType<React.ComponentProps<'svg'>>;
  description: JSX.Element;
};

const FeatureList: FeatureItem[] = [
  {
    title: "Supports SLURM, Openstack, k8s",
    Svg: require("@site/static/img/slurm_os_k8s.svg").default,
    description: (
      <>
        CEEMS was designed to be resource manager agnostic. Although
        thoeritically it can support many resource managers, we are focusing to
        support SLURM, Openstack and Kubernetes deployed on baremetal.
      </>
    ),
  },
  {
    title: "Uses eBPF for perf metrics",
    Svg: require("@site/static/img/ebpf.svg").default,
    description: (
      <>
        Besides energy and carbon footprint, CEEMS monitors and reports
        performance, IO and network metrics for user workloads using
        eBPF.
      </>
    ),
  },
  {
    title: "ML/AI workloads",
    Svg: require("@site/static/img/ml_ai.svg").default,
    description: (
      <>
        CEEMS supports energy monitoring of both NVIDIA and AMD GPUs allowing to
        quantify the energy consumption of your AI workloads.
      </>
    ),
  },
  {
    title: "Realtime emissions",
    Svg: require("@site/static/img/cloud_co2.svg").default,
    description: (
      <>
        CEEMS uses realtime emission factors to estimate the emissions of your
        workloads. It is also possible to include emboided emissions data, if
        and when available.
      </>
    ),
  },
];

function Feature({title, Svg, description}: FeatureItem) {
  return (
    <div className={clsx('col col--3')}>
      <div className="text--center">
        <Svg className={styles.featureSvg} role="img" />
      </div>
      <div className="text--center padding-horiz--md">
        <Heading as="h3">{title}</Heading>
        <p>{description}</p>
      </div>
    </div>
  );
}

export default function HomepageFeatures(): JSX.Element {
  return (
    <section className={styles.features}>
      <div className="container">
        <div className="row">
          {FeatureList.map((props, idx) => (
            <Feature key={idx} {...props} />
          ))}
        </div>
      </div>
    </section>
  );
}
